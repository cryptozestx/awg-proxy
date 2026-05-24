# awg-proxy 🔒

**CLI-утилита: AmneziaWG → SOCKS5 / HTTP прокси в пространстве пользователя**

Маршрутизируйте отдельные команды или запускайте проксированный субшелл — без root-прав, без системных изменений, без драйверов ядра.

[![Go Version](https://img.shields.io/badge/go-1.24%2B-00ADD8?logo=go)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Linux-lightgrey)](https://github.com)

---

## Как это работает

`awg-proxy` встраивает userspace-реализацию [amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) и TCP/IP стек gVisor (`netstack`) в единый бинарный файл. Утилита читает ваш существующий `.conf`-файл AmneziaWG, устанавливает зашифрованный туннель полностью в пространстве пользователя и открывает локальные прокси-серверы **SOCKS5** и **HTTP/HTTPS**.

```
Ваше приложение → локальный SOCKS5 / HTTP прокси → netstack (gVisor) → AWG туннель → VPN-сервер
```

Без `sudo`. Без сетевого интерфейса `utun`. Без изменения системной маршрутизации.

---

## Совместимость

| Версия AmneziaWG | Поддерживаемые параметры |
|---|---|
| AWG 1.x | `Jc`, `Jmin`, `Jmax`, `S1`, `S2`, `H1`–`H4` (одиночные значения) |
| AWG 2.0 | Всё выше + `S3`, `S4`, `H1`–`H4` (диапазоны), цепочки `I1`–`I5` |

---

## Установка

### Сборка из исходного кода

Требуется [Go 1.24+](https://go.dev/dl/).

```bash
git clone https://github.com/cryptozestx/awg-proxy.git
cd awg-proxy
go build -o awg-proxy .
```

### Быстрая установка

```bash
go install github.com/cryptozestx/awg-proxy@latest
```

---

## Использование

### Интерактивный проксированный субшелл (рекомендуется)

Запускает новый шелл, в котором **весь** трафик из этого окна терминала идёт через VPN:

```bash
./awg-proxy shell -c my_vpn.conf
```

Внутри субшелла используйте любые утилиты как обычно — `curl`, `git`, `npm`, `pip`, `wget` и т.д.:

```bash
# Проверьте ваш внешний IP
curl https://ipinfo.io/json

# Клонируйте репозиторий через туннель
git clone https://github.com/example/repo.git

# Введите 'exit' или нажмите Ctrl+D, чтобы закрыть туннель
exit
```

### Выполнение одной команды

```bash
./awg-proxy run -c my_vpn.conf -- curl -sL https://ipinfo.io/json
./awg-proxy run -c my_vpn.conf -- git clone https://github.com/example/repo.git
./awg-proxy run -c my_vpn.conf -- npm install
```

### Запуск конкретных macOS-приложений (только для macOS)

Вы можете запускать отдельные GUI или CLI приложения, трафик которых будет полностью маршрутизироваться через безопасный userspace VPN-туннель. Закрытие приложения автоматически отключает прокси-сервер и туннель.

```bash
# Запустить Google Chrome с отдельным изолированным безопасным профилем
./awg-proxy app -c my_vpn.conf -a "Google Chrome"

# Запустить Telegram (автоматически зарегистрирует SOCKS5 прокси в Telegram!)
./awg-proxy app -c my_vpn.conf -a Telegram

# Запустить Spotify с преднастроенными флагами проксирования
./awg-proxy app -c my_vpn.conf -a Spotify

# Запустить Slack, VS Code, Discord или любое другое Electron/GUI-приложение
./awg-proxy app -c my_vpn.conf -a Slack
./awg-proxy app -c my_vpn.conf -- "/Applications/Visual Studio Code.app"
```

#### Особенности работы с приложениями:
1. **Браузеры на базе Chromium** (`Chrome`, `Brave`, `Edge`, `Arc`): Открывает новое выделенное окно с ключом `--proxy-server`. При этом используется **изолированный и постоянный профиль** в каталоге `~/.awg-proxy/profiles/`, что позволяет проксированной сессии работать бок о бок с вашим обычным браузером и сохранять ваши куки и сессии.
2. **Telegram**: Автоматически открывает ссылку глубокого связывания `tg://socks?server=127.0.0.1&port=<port>`. Telegram сразу же предложит вам нажать **«Применить» (Enable)** для безопасного шифрования всего чат-трафика.
3. **Spotify**: Запускает плеер с встроенными флагами SOCKS5-проксирования.
4. **Остальные приложения** (Slack, Obsidian, Discord и др.): Автоматически находит исполняемый файл внутри бандла `.app` с помощью утилиты `plutil`, запускает его и пробрасывает переменные окружения `ALL_PROXY`, `HTTP_PROXY` и `HTTPS_PROXY`.

### Постоянный прокси-сервер

Привязка к фиксированным портам для браузеров и других приложений:

```bash
./awg-proxy server -c my_vpn.conf -s 1080 -h 8080
```

Затем настройте браузер или приложение на использование:
- **SOCKS5**: `127.0.0.1:1080`
- **HTTP/HTTPS**: `127.0.0.1:8080`

Для остановки нажмите `Ctrl+C`.

---

## Конфигурация

`awg-proxy` читает стандартные `.conf`-файлы WireGuard / AmneziaWG. Полный пример с аннотациями смотрите в [`example.conf`](example.conf).

> ⚠️ **Никогда не коммитьте свой реальный `.conf`-файл.** Он содержит ваш приватный ключ. `.gitignore` в этом репозитории уже исключает все `*.conf`-файлы, кроме `example.conf`.

---

## Справка по CLI

```
Использование:
  awg-proxy <команда> [параметры]

Команды:
  shell    Запустить прокси и открыть интерактивный субшелл
  run      Запустить прокси, выполнить одну команду и выйти
  app      Запустить прокси, открыть конкретное macOS-приложение и завершить при закрытии
  server   Запустить постоянный прокси-сервер в активном окне

Параметры:
  -c, --config       Путь к .conf-файлу AmneziaWG (обязательный)
  -a, --app          Имя или путь приложения macOS (только для команды 'app')
  -s, --socks-port   Порт для SOCKS5 (по умолчанию: авто)
  -h, --http-port    Порт для HTTP-прокси (по умолчанию: авто)
  -d, --debug        Подробное логирование туннеля
```

---

## Безопасность

- Приватный ключ никогда не покидает ваш компьютер — туннель устанавливается локально.
- Только трафик, который вы явно направляете через прокси (`shell`, `run`) или настраиваете в приложениях (`server`), проходит через VPN. Системная маршрутизация не изменяется.
- Не публикуйте и не коммитьте ваш `.conf`-файл.

---

## Участие в разработке

Мы рады вашему вкладу! Пожалуйста, ознакомьтесь с [CONTRIBUTING.md](CONTRIBUTING.md) перед тем, как открывать PR.

---

## Лицензия

[MIT](LICENSE) — подробности в файле.

---

## Благодарности

- [amnezia-vpn/amneziawg-go](https://github.com/amnezia-vpn/amneziawg-go) — userspace-реализация AmneziaWG
- [google/gvisor](https://github.com/google/gvisor) — userspace TCP/IP стек (netstack)
