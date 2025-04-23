package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
)

const configPath = "config.json"

// Config stored on disk
type Config struct {
	DiscordChannelID string `json:"discord_channel_id"`
	TelegramChatID   int64  `json:"telegram_chat_id"`
	// map[DiscordDisplay] = "@telegramHandle"
	UserMap map[string]string `json:"user_map"`
}

type MessageRef struct {
	ChannelID string
	MessageID string
}

var (
	config              Config
	configMu            sync.Mutex
	discordToTelegram   = make(map[string]int)     // key "<channelID>:<discordMsgID>" → TG msgID
	telegramToDiscord   = make(map[int]MessageRef) // key TG msgID → Discord ref
	bridgeMappingsMutex sync.Mutex
)

// --------------------------------------------------
// helpers
// --------------------------------------------------
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

// --------------------------------------------------
// main
// --------------------------------------------------
func main() {
	_ = godotenv.Load()

	discordToken := os.Getenv("DISCORD_TOKEN")
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if discordToken == "" || telegramToken == "" {
		log.Fatal("DISCORD_TOKEN or TELEGRAM_TOKEN not set in environment")
	}

	// load config or create new
	if err := loadConfig(); err != nil {
		log.Printf("⚠️  Could not load %s: %v — creating new", configPath, err)
		saveConfig()
	}
	if config.UserMap == nil {
		config.UserMap = make(map[string]string)
	}

	// Telegram bot
	tgBot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatalf("Telegram init error: %v", err)
	}
	log.Printf("TG bot: @%s", tgBot.Self.UserName)

	// Discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Discord session error: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	//--------------------------------------------------
	// Discord → Telegram
	//--------------------------------------------------
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}

		content := strings.TrimSpace(m.Content)

		// /syn command binds channel
		if strings.HasPrefix(strings.ToLower(content), "/syn") {
			configMu.Lock()
			config.DiscordChannelID = m.ChannelID
			configMu.Unlock()
			saveConfig()
			s.ChannelMessageSend(m.ChannelID, "✅ Discord channel registered")
			return
		}

		// ensure binding
		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()
		if dCh == "" || tCh == 0 {
			return
		}

		// threads filter
		channel, err := s.Channel(m.ChannelID)
		if err != nil {
			return
		}
		isThread := channel.Type == discordgo.ChannelTypeGuildPublicThread || channel.Type == discordgo.ChannelTypeGuildPrivateThread || channel.Type == discordgo.ChannelTypeGuildNewsThread
		if (!isThread && m.ChannelID != dCh) || (isThread && channel.ParentID != dCh) {
			return
		}

		// display name
		member, _ := s.GuildMember(m.GuildID, m.Author.ID)
		displayName := firstNonEmpty(member.Nick, m.Author.GlobalName, m.Author.Username)

		configMu.Lock()
		tgHandle := config.UserMap[displayName] // may be "@user"
		configMu.Unlock()

		prefix := ""
		if isThread {
			prefix = "[Thread: " + channel.Name + "] "
		}
		body := "*" + displayName + "*: " + prefix + content
		if tgHandle != "" {
			body += " " + tgHandle
		}

		msgCfg := tgbotapi.NewMessage(tCh, body)
		msgCfg.ParseMode = "Markdown"

		if m.MessageReference != nil {
			key := m.MessageReference.ChannelID + ":" + m.MessageReference.MessageID
			bridgeMappingsMutex.Lock()
			if tgID, ok := discordToTelegram[key]; ok {
				msgCfg.ReplyToMessageID = tgID
			}
			bridgeMappingsMutex.Unlock()
		}

		tgMsg, err := tgBot.Send(msgCfg)
		if err != nil {
			log.Printf("Error sending to TG: %v", err)
			return
		}

		// store mappings both ways
		key := m.ChannelID + ":" + m.ID
		bridgeMappingsMutex.Lock()
		discordToTelegram[key] = tgMsg.MessageID
		telegramToDiscord[tgMsg.MessageID] = MessageRef{ChannelID: m.ChannelID, MessageID: m.ID}
		bridgeMappingsMutex.Unlock()
	})

	if err := dg.Open(); err != nil {
		log.Fatalf("Cannot connect to Discord: %v", err)
	}
	defer dg.Close()
	log.Println("Discord connected…")

	//--------------------------------------------------
	// Telegram → Discord
	//--------------------------------------------------
	updates := tgBot.GetUpdatesChan(tgbotapi.UpdateConfig{Timeout: 60})

	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := strings.TrimSpace(update.Message.Text)
		chatID := update.Message.Chat.ID

		// /ack binds TG chat
		if strings.HasPrefix(strings.ToLower(text), "/ack") {
			configMu.Lock()
			config.TelegramChatID = chatID
			configMu.Unlock()
			saveConfig()
			tgBot.Send(tgbotapi.NewMessage(chatID, "✅ Telegram chat registered"))
			continue
		}

		// ensure bound chat
		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()
		if dCh == "" || tCh == 0 || chatID != tCh {
			continue
		}

		// get TG handle
		tgHandle := ""
		if update.Message.From != nil {
			if update.Message.From.UserName != "" {
				tgHandle = "@" + update.Message.From.UserName
			} else {
				tgHandle = update.Message.From.FirstName
			}
		}

		// map to Discord display
		discordName := tgHandle
		configMu.Lock()
		for dName, handle := range config.UserMap {
			if handle == tgHandle {
				discordName = dName
				break
			}
		}
		configMu.Unlock()

		formatted := "**" + discordName + "**: " + text

		// handle replies (including self‑reply chains)
		if update.Message.ReplyToMessage != nil {
			origID := update.Message.ReplyToMessage.MessageID
			bridgeMappingsMutex.Lock()
			ref, ok := telegramToDiscord[origID]
			bridgeMappingsMutex.Unlock()
			if ok {
				sentMsg, err := dg.ChannelMessageSendReply(
					ref.ChannelID,
					formatted,
					&discordgo.MessageReference{MessageID: ref.MessageID, ChannelID: ref.ChannelID},
				)
				if err != nil {
					log.Printf("Error sending reply to Discord: %v", err)
				} else {
					bridgeMappingsMutex.Lock()
					telegramToDiscord[update.Message.MessageID] = MessageRef{ChannelID: sentMsg.ChannelID, MessageID: sentMsg.ID}
					bridgeMappingsMutex.Unlock()
				}
				continue
			}
		}

		// normal forward
		dsMsg, err := dg.ChannelMessageSend(dCh, formatted)
		if err != nil {
			log.Printf("Error sending to Discord: %v", err)
			continue
		}
		bridgeMappingsMutex.Lock()
		telegramToDiscord[update.Message.MessageID] = MessageRef{ChannelID: dCh, MessageID: dsMsg.ID}
		bridgeMappingsMutex.Unlock()
	}
}

// --------------------------------------------------
// util
// --------------------------------------------------
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
