# Sudoku UI

Минимальная панель для официального [SUDOKU-ASCII/sudoku](https://github.com/SUDOKU-ASCII/sudoku).

Принцип проекта: **один VPS = одна панель = один сервер Sudoku**. После входа пользователь создаёт подключение, выпускает отдельные ключи для устройств и получает QR, `sudoku://`-ссылку или параметры для ручной настройки.

## One-click установка

Запустите от `root` на Ubuntu или Debian (amd64/arm64):

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/sanderstripa/sudoku-ui/main/install.sh)
```

Установщик не создаёт Sudoku-подключение. Он устанавливает стабильные релизы панели и Core, создаёт службы systemd, настраивает Caddy с автоматически обновляемым HTTPS-сертификатом и печатает уникальный адрес, логин и пароль. Внешние порты `80` и `443` VPS должны быть открыты. Если Sudoku уже найден, установщик спрашивает, использовать его, заменить бинарник с резервной копией или отменить установку.

## Возможности

- один экран без бокового меню и Dashboard;
- безопасные настройки по умолчанию и справка `?` у параметров;
- AEAD, четыре режима таблицы, padding, Pure Downlink, HTTP Mask и Multiplex;
- штатная генерация Master/Available/Split ключей Sudoku;
- QR, ссылка и ручные параметры подключения;
- компактные показатели CPU, RAM, диска и трафика;
- журнал панели и Core в одном окне;
- только ручная проверка и установка обновлений;
- bcrypt, HttpOnly/SameSite cookie, CSRF-защита и ограничение попыток входа.

## Разработка

```bash
go test ./...
go vet ./...
go build -o sudoku-ui .
node --check web/app.js
bash -n install.sh
```

Frontend встроен в Go-бинарник через `embed.FS`.

## Файлы на сервере

```text
/usr/local/bin/sudoku-ui
/usr/local/bin/sudoku
/etc/sudoku-ui/config.json
/etc/sudoku-ui/state.json
/etc/sudoku/config.json
/etc/systemd/system/sudoku-ui.service
/etc/systemd/system/sudoku.service
```

Приватные ключи хранятся локально с правами `0600` и не возвращаются общим API состояния. Удаление экспортированного клиентского ключа из панели не отзывает его криптографически: для настоящего отзыва необходимо сменить Master Key подключения.

Sudoku UI не является частью проекта SUDOKU-ASCII.
