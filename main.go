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
	DiscordChannelID string `json:"discord_channel_id"`
	TelegramChatID   int64  `json:"telegram_chat_id"`
}

var (
	config   Config
	configMu sync.Mutex
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

func PrintUserNames(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil {
		log.Println("Нет информации об авторе")
		return
	}

	username := m.Author.Username
	globalName := m.Author.GlobalName

	member, err := s.GuildMember(m.GuildID, m.Author.ID)
	if err != nil {
		log.Printf("Ошибка получения участника сервера: %v", err)
		return
	}
	nickname := member.Nick

	displayName := nickname
	if displayName == "" {
		displayName = globalName
	}
	if displayName == "" {
		displayName = username
	}

	log.Println("====== User Info ======")
	log.Printf("Username: %s", username)
	log.Printf("Global Display Name: %s", globalName)
	log.Printf("Server Nickname: %s", nickname)
	log.Printf("Chosen Display Name: %s", displayName)
	log.Println("========================")
}

func main() {
	// Load static tokens
	_ = godotenv.Load()

	discordToken := os.Getenv("DISCORD_TOKEN")
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if discordToken == "" || telegramToken == "" {
		log.Fatal("DISCORD_TOKEN or TELEGRAM_TOKEN not set in environment")
	}

	// Load or initialize config.json
	if err := loadConfig(); err != nil {
		log.Printf("⚠️ Could not load %s: %v. Creating new.", configPath, err)
		if err := saveConfig(); err != nil {
			log.Fatalf("❌ Could not create %s: %v", configPath, err)
		}
	}

	// Initialize Telegram bot
	tgBot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatalf("Telegram init error: %v", err)
	}
	log.Printf("Telegram bot: %s", tgBot.Self.UserName)

	// Initialize Discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Discord session error: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent

	// Discord -> Telegram handler
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}
		PrintUserNames(s, m) // new add 18:30

		content := strings.TrimSpace(m.Content)

		// /syn command
		if strings.HasPrefix(strings.ToLower(content), "/syn") {
			configMu.Lock()
			config.DiscordChannelID = m.ChannelID
			configMu.Unlock()
			if err := saveConfig(); err != nil {
				s.ChannelMessageSend(m.ChannelID, "Error saving configuration.")
				return
			}
			s.ChannelMessageSend(m.ChannelID, "✅ Discord channel registered: "+m.ChannelID)
			return
		}

		// Forward after registration
		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()

		if dCh != "" && tCh != 0 && m.ChannelID == dCh {
			raw := "**" + m.Author.Username + "** 	: " + content
			msg := tgbotapi.NewMessage(tCh, raw)
			if _, err := tgBot.Send(msg); err != nil {
				log.Printf("Error sending to Telegram: %v", err)
			} else {
				log.Printf("→ TG: %s", raw)
			}
		}
	})

	// Open Discord connection
	if err := dg.Open(); err != nil {
		log.Fatalf("Could not connect to Discord: %v", err)
	}
	defer dg.Close()
	log.Println("Discord connected...")

	// Telegram -> Discord loop
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, _ := tgBot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := strings.TrimSpace(update.Message.Text)
		chatID := update.Message.Chat.ID

		// DEBUG logs
		log.Printf("📨 TG update — incoming chatID=%d, saved chatID=%d, text=%q", chatID, config.TelegramChatID, text)
		log.Printf("DBG ▶ TG→DS — incoming chatID=%d, saved=%d, from=%s, text=%q",
			chatID, config.TelegramChatID, update.Message.From.UserName, text)

		// /ack command
		if strings.HasPrefix(strings.ToLower(text), "/ack") {
			configMu.Lock()
			config.TelegramChatID = chatID
			configMu.Unlock()
			if err := saveConfig(); err != nil {
				tgBot.Send(tgbotapi.NewMessage(chatID, "Error saving configuration."))
				continue
			}
			tgBot.Send(tgbotapi.NewMessage(chatID, "✅ Telegram chat registered: "+strconv.FormatInt(chatID, 10)))
			continue
		}

		// Forward after registration
		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()

		if dCh != "" && tCh != 0 && chatID == tCh {
			discordText := update.Message.From.UserName + ": " + text
			if _, err := dg.ChannelMessageSend(dCh, discordText); err != nil {
				log.Printf("Error sending to Discord: %v", err)
			} else {
				log.Printf("→ DS: %s", discordText)
			}
		}
	}
}
