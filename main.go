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

const (
	configPath = "config.json"
	devCode    = "SECRET123" // код разработчика, можно убрать проверку или сделать другим способом
)

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

func main() {
	// 1) Загрузка .env
	_ = godotenv.Load()

	discordToken := os.Getenv("DISCORD_TOKEN")
	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if discordToken == "" || telegramToken == "" {
		log.Fatal("DISCORD_TOKEN или TELEGRAM_TOKEN не заданы в окружении")
	}

	// 2) Загрузка (или инициализация) config.json
	if err := loadConfig(); err != nil {
		log.Printf("⚠️ Не удалось загрузить %s: %v. Будет создан новый.", configPath, err)
		if err := saveConfig(); err != nil {
			log.Fatalf("❌ Не удалось создать %s: %v", configPath, err)
		}
	}

	// 3) Инициализация Telegram
	tgBot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatalf("Ошибка инициализации Telegram: %v", err)
	}
	log.Printf("Telegram бот: %s", tgBot.Self.UserName)

	// 4) Инициализация Discord
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Ошибка создания Discord-сессии: %v", err)
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsMessageContent

	// 5) Discord-обработчик
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author.Bot {
			return
		}
		content := strings.TrimSpace(m.Content)

		// 5.1) Команда /syn — регистрируем Discord-канал
		if strings.HasPrefix(strings.ToLower(content), "/syn") {
			configMu.Lock()
			config.DiscordChannelID = m.ChannelID
			configMu.Unlock()
			if err := saveConfig(); err != nil {
				s.ChannelMessageSend(m.ChannelID, "Ошибка сохранения конфигурации.")
				return
			}
			s.ChannelMessageSend(m.ChannelID, "✅ Discord‑канал зарегистрирован: "+m.ChannelID)
			return
		}

		// 5.2) После регистрации обоих ID пересылаем в Telegram
		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()

		if dCh != "" && tCh != 0 && m.ChannelID == dCh {
			text := "**" + m.Author.Username + "**: " + m.Content
			msg := tgbotapi.NewMessage(tCh, text)
			//msg.ParseMode = "Markdown"
			if _, err := tgBot.Send(msg); err != nil {
				log.Printf("Ошибка отправки в Telegram: %v", err)
			}
		}
	})

	// 6) Подключаемся к Discord
	if err := dg.Open(); err != nil {
		log.Fatalf("Не удалось подключиться к Discord: %v", err)
	}
	defer dg.Close()
	log.Println("Discord подключен…")

	// 7) Обновления Telegram
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, _ := tgBot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}
		text := strings.TrimSpace(update.Message.Text)
		chatID := update.Message.Chat.ID

		// DEBUG: смотрим, какие ID сравниваются
		log.Printf("📨 TG update — incoming chatID=%d, saved chatID=%d, text=%q",
			chatID, config.TelegramChatID, text)

		// 7.1) Команда /ack — регистрируем Telegram‑чат
		if strings.HasPrefix(strings.ToLower(text), "/ack") {
			configMu.Lock()
			config.TelegramChatID = chatID
			configMu.Unlock()
			if err := saveConfig(); err != nil {
				tgBot.Send(tgbotapi.NewMessage(chatID, "Ошибка сохранения конфигурации."))
				continue
			}
			tgBot.Send(tgbotapi.NewMessage(chatID, "✅ Telegram‑чат зарегистрирован: "+strconv.FormatInt(chatID, 10)))
			continue
		}

		// 7.2) После регистрации обоих ID пересылаем в Discord
		configMu.Lock()
		dCh := config.DiscordChannelID
		tCh := config.TelegramChatID
		configMu.Unlock()

		if dCh != "" && tCh != 0 && chatID == tCh {
			discordText := update.Message.From.UserName + ": " + text
			if _, err := dg.ChannelMessageSend(dCh, discordText); err != nil {
				log.Printf("Ошибка отправки в Discord: %v", err)
			}
		}
	}
}
