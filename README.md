# Sudoku UI

Веб-панель для управления VPN на протоколе **Sudoku** (аналог 3x-ui).

## Возможности MVP

- ✅ One-click установка (`install.sh`)
- ✅ Веб-панель с авторизацией (JWT + bcrypt)
- ✅ Создание / удаление клиентов
- ✅ Генерация `sudoku://` ссылок + QR-код
- ✅ Обновление ядра Sudoku одной кнопкой
- ✅ Управление через systemd

## Быстрый старт (разработка)

```bash
# Зависимости уже в go.mod
go build -o /tmp/sudoku-panel ./cmd/panel
/tmp/sudoku-panel
```

Открой http://localhost:2053  
Логин: `admin` / Пароль: `admin`

## Установка на VPS

```bash
# После публикации репозитория:
sudo bash -c "$(curl -fsSL https://raw.githubusercontent.com/YOUR_REPO/sudoku-ui/main/install.sh)"

# Или локально:
sudo bash install.sh
```

После установки:
- Панель: `http://IP:2053`
- Логин/пароль: `admin` / `admin` (смени сразу!)

## Структура

```
cmd/panel/          — точка входа + frontend
internal/
  auth/             — JWT + bcrypt
  config/           — пути и константы
  db/               — SQLite
  models/           — структуры
  sudoku/           — ключи, сервис, обновление ядра
web/                — исходник index.html
install.sh          — one-click installer
```

## API (кратко)

| Метод | Путь | Описание |
|-------|------|----------|
| POST | /api/login | Вход |
| GET | /api/status | Статус |
| GET/POST | /api/clients | Список / создать |
| DELETE | /api/clients/:id | Удалить |
| GET | /api/clients/:id/link | Ссылка + QR данные |
| POST | /api/update-core | Обновить ядро Sudoku |
| POST | /api/change-password | Сменить пароль |

## Дальнейшее развитие

- Лимиты трафика / срок действия
- Multi-node
- Полноценный Vue/React фронт
- Подписки (subscription URL)

---

Сделано в стиле vibe-coding.
