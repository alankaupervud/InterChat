# InterChat

**Интеграционный бот на Go для двусторонней пересылки сообщений между Discord и Telegram.**

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

---

## Содержание

- [Описание](#описание)
- [Установка](#установка)
- [Конфигурация](#конфигурация)
- [Использование](#использование)
- [Поддержка user_map](#поддержка-user_map)
- [Команды](#команды)
- [Разработка](#разработка)
- [Лицензия](#лицензия)

---

## Описание

`InterChat` — это лёгкий бот на Go, который:

- **Пересылает** сообщения из указанного канала Discord в группу или канал Telegram.
- **Пересылает** сообщения из указанной группы/чата Telegram в канал Discord.
- **Динамически** настраивает целевые каналы через команды `/syn` и `/ack`.
- **Поддерживает сопоставление имён пользователей через user_map** для корректного отображения ников между платформами.

Проект на GitHub: [https://github.com/alankaupervud/InterChat](https://github.com/alankaupervud/InterChat)

---

## Установка

```bash
# Клонируем репозиторий
git clone https://github.com/alankaupervud/InterChat.git
cd InterChat

# Загружаем зависимости
go mod tidy
```

---

## Конфигурация

### 1. .env (статичные переменные)

Создайте файл `.env` в корне проекта со следующим содержимым:

```dotenv
DISCORD_TOKEN=ваш_токен_дискорд_бота
TELEGRAM_TOKEN=ваш_токен_телеграм_бота
```

### 2. config.json (динамические ID и user_map)

При первом запуске создаётся файл `config.json`. Его структура:

```json
{
  "discord_channel_id": "",
  "telegram_chat_id": 0,
  "user_map": {
    "Alan Trump": "@alan",
    "Jane Doe": "@janey",
    "John Smith": "@jsmith"
  }
}
```

Поля:
- `discord_channel_id` и `telegram_chat_id` — автоматически заполняются командами `/syn` и `/ack`.
- `user_map` — сопоставление отображаемых имён из Discord с никами Telegram.

---

## Использование

```bash
go run main.go
```

1. В Discord: в нужном канале введите:
   ```
   /syn
   ```
2. В Telegram-группе с ботом:
   ```
   /ack
   ```
3. Бот начнёт пересылку сообщений между платформами.

> ⚠️ Обратите внимание:
> - В Discord Developer Portal включите **Message Content Intent**.
> - В BotFather отключите **Privacy Mode** (`/setprivacy` → Disable).

---

## Поддержка `user_map`

Функция `user_map` позволяет сопоставить реальные имена пользователей Discord с их Telegram-никами.

### Пример:
```json
"user_map": {
  "Name Surnamev": "@Nickname"
}
```

Если в Discord приходит сообщение от `Name Surname`, то оно будет переслано в Telegram как:
```
@Nickname: Hello!
```

Если пользователь не найден в `user_map`, то используется исходное имя.

---

## Команды

| Платформа | Команда | Описание                                 |
|-----------|---------|------------------------------------------|
| Discord   | `/syn`  | Сохранить текущий канал в `config.json`  |
| Telegram  | `/ack`  | Сохранить текущий чат в `config.json`    |

---

## Разработка

Форматирование кода:
```bash
go fmt ./...
```

Линтинг:
```bash
go vet ./...
```

---

## Лицензия

Проект выпущен под лицензией MIT.  
См. файл [LICENSE](LICENSE).

