package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
)

const configPath = "config.json"

type Config struct {
	DiscordChannelID string            `json:"discord_channel_id"`
	TelegramChatID   int64             `json:"telegram_chat_id"`
	UserMap          map[string]string `json:"user_map"`
}

type MessageRef struct {
	ChannelID string
	MessageID string
}

var (
	config              Config
	configMu            sync.Mutex
	discordToTelegram   = make(map[string]int)
	telegramToDiscord   = make(map[string]MessageRef)
	bridgeMappingsMutex sync.Mutex
)

func loadConfig() error {
	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		return err
	}
	configMu.Lock()
	defer configMu.Unlock()
	return json.Unmarshal(data, &config)
}

func saveConfig() error {
	configMu.Lock()
	defer configMu.Unlock()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(configPath, data, 0644)
}

func main() {
	_ = godotenv.Load()

	discordToken := os.Getenv("DISCORD_TOKEN")
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if discordToken == "" || telegramToken == "" {
		log.Fatal("DISCORD_TOKEN or TELEGRAM_TOKEN not set in environment")
	}

	if err := loadConfig(); err != nil {
		log.Printf("⚠️ Could not load %s: %v. Creating new.", configPath, err)
		if err := saveConfig(); err != nil {
			log.Fatalf("❌ Could not create %s: %v", configPath, err)
		}
	}

	tgBot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatalf("Telegram init error: %v", err)
	}
	log.Printf("Telegram bot: %s", tgBot.Self.UserName)

	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Discord session error: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}

		content := strings.TrimSpace(m.Content)
		if strings.HasPrefix(strings.ToLower(content), "/syn") {
			configMu.Lock()
			config.DiscordChannelID = m.ChannelID
			configMu.Unlock()
			saveConfig()
			s.ChannelMessageSend(m.ChannelID, "✅ Discord channel registered: "+m.ChannelID)
			return
		}

		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()
		if dCh == "" || tCh == 0 {
			return
		}

		channel, err := s.Channel(m.ChannelID)
		if err != nil {
			return
		}
		isThread := channel.Type == discordgo.ChannelTypeGuildPublicThread ||
			channel.Type == discordgo.ChannelTypeGuildPrivateThread ||
			channel.Type == discordgo.ChannelTypeGuildNewsThread

		if !isThread && m.ChannelID != dCh {
			return
		}
		if isThread && channel.ParentID != dCh {
			return
		}

		username := m.Author.Username
		globalName := m.Author.GlobalName
		member, _ := s.GuildMember(m.GuildID, m.Author.ID)
		nick := ""
		if member != nil {
			nick = member.Nick
		}
		displayName := nick
		if displayName == "" {
			displayName = globalName
		}
		if displayName == "" {
			displayName = username
		}

		configMu.Lock()
		tgNick := config.UserMap[displayName]
		configMu.Unlock()

		threadInfo := ""
		if isThread {
			threadInfo = "[Thread: " + channel.Name + "] "
		}

		raw := "*" + displayName + "*: " + threadInfo + m.Content
		if tgNick != "" {
			raw += "  " + tgNick
		}

		var tgMsg tgbotapi.Message
		if m.MessageReference != nil {
			key := m.ChannelID + ":" + m.MessageReference.MessageID
			bridgeMappingsMutex.Lock()
			ref, ok := discordToTelegram[key]
			bridgeMappingsMutex.Unlock()
			if ok {
				msg := tgbotapi.NewMessage(tCh, raw)
				msg.ReplyToMessageID = ref
				tgMsg, _ = tgBot.Send(msg)
			} else {
				tgMsg, _ = tgBot.Send(tgbotapi.NewMessage(tCh, raw))
			}
		} else {
			tgMsg, _ = tgBot.Send(tgbotapi.NewMessage(tCh, raw))
		}

		if tgMsg.MessageID != 0 {
			key := m.ChannelID + ":" + m.ID
			bridgeMappingsMutex.Lock()
			discordToTelegram[key] = tgMsg.MessageID
			bridgeMappingsMutex.Unlock()
		}
	})

	if err := dg.Open(); err != nil {
		log.Fatalf("Could not connect to Discord: %v", err)
	}
	defer dg.Close()

	log.Println("Discord connected…")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, _ := tgBot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := strings.TrimSpace(update.Message.Text)
		chatID := update.Message.Chat.ID

		if strings.HasPrefix(strings.ToLower(text), "/ack") {
			configMu.Lock()
			config.TelegramChatID = chatID
			configMu.Unlock()
			saveConfig()
			tgBot.Send(tgbotapi.NewMessage(chatID, "✅ Telegram chat registered: "+strconv.FormatInt(chatID, 10)))
			continue
		}

		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()
		if dCh == "" || tCh == 0 || chatID != tCh {
			continue
		}

		if update.Message.ReplyToMessage != nil {
			prevID := update.Message.ReplyToMessage.MessageID
			bridgeMappingsMutex.Lock()
			ref := telegramToDiscord[strconv.Itoa(prevID)]
			bridgeMappingsMutex.Unlock()
			dg.ChannelMessageSendReply(ref.ChannelID, text, &discordgo.MessageReference{
				MessageID: ref.MessageID,
				ChannelID: ref.ChannelID,
			})
		} else {
			dsMsg, _ := dg.ChannelMessageSend(dCh, text)
			bridgeMappingsMutex.Lock()
			telegramToDiscord[dsMsg.ID] = MessageRef{ChannelID: dCh, MessageID: dsMsg.ID}
			bridgeMappingsMutex.Unlock()
		}
	}
}
