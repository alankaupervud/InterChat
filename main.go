package main

import (
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func init() {
	// Загружаем переменные окружения из .env файла
	if err := godotenv.Load(); err != nil {
		log.Println("⚠️ Не удалось загрузить .env файл, используем системные переменные окружения.")
	}
}

func main() {
	discordToken := os.Getenv("DISCORD_TOKEN")
	if discordToken == "" {
		log.Fatal("DISCORD_TOKEN не задан")
	}

	// Инициализируем Discord-сессию
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		log.Fatal("Ошибка инициализации Discord:", err)
	}

	// Регистрируем обработчик событий создания сообщений
	dg.AddHandler(messageCreateHandler)

	// Открываем соединение с Discord
	err = dg.Open()
	if err != nil {
		log.Fatal("Не удалось установить соединение с Discord:", err)
	}
	defer dg.Close()

	log.Println("Discord-связь установлена. Бот слушает сообщения...")
	select {}
}

// messageCreateHandler обрабатывает события создания сообщений
func messageCreateHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Пропускаем сообщения, отправленные ботами
	if m.Author.Bot {
		return
	}
	log.Printf("Канал: %s | Автор: %s | Сообщение: %s", m.ChannelID, m.Author.Username, m.Content)
}
