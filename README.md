# Sudoku UI

Веб-панель для управления VPN-протоколом [Sudoku](https://github.com/SUDOKU-ASCII/sudoku).

Аналог 3x-ui, но для Sudoku: создание клиентов, генерация ключей, `sudoku://` ссылок и полных учётных данных.

## One-Click Install

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/sanderstripa/sudoku-ui/main/install.sh)
```

После установки:

| | |
|---|---|
| **Панель** | `http://IP:2053` |
| **Логин** | `admin` |
| **Пароль** | `admin` |
| **VPN порт** | `44300` |

> Смени пароль после первого входа!

## Что умеет

- Создание / удаление клиентов (инбаундов)
- Генерация split private keys от master key
- `sudoku://` short links + QR
- Полные учётные данные (адрес, порт, ключ, метод, ascii mode, padding…)
- Обновление ядра Sudoku одной кнопкой
- Systemd: автозапуск панели и ядра

## Управление

```bash
systemctl status sudoku-panel
systemctl status sudoku-core
systemctl restart sudoku-panel
systemctl restart sudoku-core

# логи
journalctl -u sudoku-panel -f
journalctl -u sudoku-core -f
```

## Ручная установка

```bash
git clone https://github.com/sanderstripa/sudoku-ui.git
cd sudoku-ui
# нужен Go 1.21+
go build -o bin/sudoku-panel ./cmd/panel
# скачать ядро Sudoku в data/bin/sudoku
./bin/sudoku-panel
```

## Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `PANEL_PORT` | `2053` | Порт веб-панели |
| `SUDOKU_PORT` | `44300` | Порт VPN (в install.sh) |
| `DATA_DIR` | `./data` | Каталог данных |

## Структура

```
sudoku-ui/
├── cmd/panel/          # точка входа панели
├── internal/
│   ├── auth/           # JWT
│   ├── db/             # SQLite
│   ├── sudoku/         # ключи, ссылки, update core
│   └── ...
├── install.sh          # one-click
└── data/               # runtime (ключи, бинарник, БД)
```

## Лицензия

MIT
