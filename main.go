package main

import (
	"log"
	"os"
	"strconv"

	"github.com/bwmarrin/discordgo"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/joho/godotenv"
)

func init() {
	// Загружаем переменные окружения из файла .env (если он есть)
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ .env файл не найден, используются системные переменные")
	}
}

func main() {
	// Чтение переменных окружения
	discordToken := os.Getenv("DISCORD_TOKEN")
	if discordToken == "" {
		log.Fatal("Нет DISCORD_TOKEN")
	}

	telegramToken := os.Getenv("TELEGRAM_TOKEN")
	if telegramToken == "" {
		log.Fatal("Нет TELEGRAM_TOKEN")
	}

	chatIDStr := os.Getenv("TELEGRAM_CHAT_ID")
	if chatIDStr == "" {
		log.Fatal("Нет TELEGRAM_CHAT_ID")
	}
	telegramChatID, err := strconv.ParseInt(chatIDStr, 10, 64)
	if err != nil {
		log.Fatal("Ошибка парсинга TELEGRAM_CHAT_ID:", err)
	}

	// Инициализация Telegram-бота
	tgBot, err := tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatalf("Ошибка инициализации Telegram: %v", err)
	}
	log.Printf("Telegram бот запущен: %s", tgBot.Self.UserName)

	// Инициализация Discord-сессии
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatalf("Ошибка создания Discord сессии: %v", err)
	}

	// Включаем необходимые intents, чтобы получать содержимое сообщений
	dg.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentsMessageContent

	// Обработчик сообщений из Discord

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// Игнорируем сообщения, отправленные ботами
		if m.Author.Bot {
			return
		}

		// Получаем информацию о канале по его ID
		channel, err := s.Channel(m.ChannelID)
		var channelName string
		if err != nil {
			log.Printf("Ошибка получения информации о канале: %v", err)
			// Если возникает ошибка — можно оставить id в качестве имени канала
			channelName = m.ChannelID
		} else {
			channelName = channel.Name
		}

		// Выводим в консоль: имя канала, имя отправителя и текст сообщения
		log.Printf("Канал: %s | Автор: %s | Сообщение: %s", channelName, m.Author.Username, m.Content)

		// Форматируем сообщение для Telegram, включаем имя канала
		telegramText := "**" + m.Author.Username + "** (**" + channelName + "**): " + m.Content

		msg := tgbotapi.NewMessage(telegramChatID, telegramText)
		msg.ParseMode = "Markdown"

		if _, err := tgBot.Send(msg); err != nil {
			log.Printf("Ошибка отправки сообщения в Telegram: %v", err)
		} else {
			log.Printf("Сообщение успешно отправлено в Telegram: %s", telegramText)
		}
	})

	// Подключаемся к Discord
	err = dg.Open()
	if err != nil {
		log.Fatalf("Ошибка подключения к Discord: %v", err)
	}
	defer dg.Close()

	log.Println("Бот работает. Ожидаем сообщения из Discord для пересылки в Telegram...")
	select {} // Бесконечный цикл, чтобы приложение не завершилось
}
