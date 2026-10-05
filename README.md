# tatnet-mcp

MCP-сервер TatNet: управление платформой из Claude (claude.ai, Desktop, Code),
ChatGPT и любого клиента [Model Context Protocol](https://modelcontextprotocol.io).
«Сделай сайт и выложи на TatNet» — это вызов `deploy_files`.

## Quick start / English

TatNet MCP is a remote server for deploying static and SSR websites from an AI
conversation and managing Git/Docker apps, builds, logs, environment variables
and domains on TatNet, a Russian cloud platform. Transport: Streamable HTTP.
Endpoint: `https://mcp.tatnet.cloud/mcp`. Authentication: OAuth or a scoped API key.

```bash
claude mcp add --transport http tatnet https://mcp.tatnet.cloud/mcp
```

Connect through OAuth, choose a project and write access, then ask the assistant
to publish your website. A deployment is ready when both `status=success` and
`deploy_state=live` are confirmed. Backend apps require Git or a Docker image;
VMs, databases, DNS zones and S3 use the CLI/API. App deletion and balance top-ups
are unavailable through MCP.

- [MCP overview](https://tatnet.ru/mcp) and [connection docs](https://docs.tatnet.ru/docs/mcp).
- [Publish from Claude Code](https://docs.tatnet.ru/docs/guides/en/claude-code-deploy).
- [Diagnose a failed build](https://docs.tatnet.ru/docs/guides/en/debug-build).
- [CLI deployment from CI](https://docs.tatnet.ru/docs/guides/en/cli-ci-deploy).

## Устройство

- **Тонкий слой над публичным `/v1`.** Своей БД, своих прав и бизнес-логики нет:
  каждый инструмент зовёт `api.tatnet.ru/v1` через `tatnet-go` ключом того, кто
  подключился. Права — политика ключа ∩ живая роль создателя, как у CLI и SDK.
  Чего нет в `/v1`, того нет и здесь: сначала ручка в `/v1`, потом инструмент.
- **Streamable HTTP без состояния** (`/mcp`), официальный
  `modelcontextprotocol/go-sdk`. Реплики взаимозаменяемы, выкат клиентов не рвёт.
- **Авторизация — OAuth или ключ `/v1`.** Клиент без поля для заголовка
  регистрируется сам (DCR), человек входит в TatNet и на экране согласия
  выбирает аккаунт, проекты и уровень доступа; подключение появляется в панели
  рядом с API-ключами и отзывается там же. Токен выдан только для MCP и дальше
  сервера не уходит — в `/v1` сервер ходит сам от имени подключения. Клиенты,
  которые умеют передать заголовок, могут прийти и с ключом
  `Authorization: Bearer tn_live_…`. Права в обоих случаях решает api на каждом
  вызове: политика подключения или ключа пересекается с ролью человека.

## Инструменты (приложения и баланс)

| Инструмент | Тип | Что делает |
|---|---|---|
| `whoami` | чтение | аккаунт и проекты, ограничен ли ключ |
| `get_balance` | чтение | баланс: доступно сейчас, деньги, бонусы и открыты ли они. Только владельцу: ключу нужно `billing:read`, OAuth-подключению — выдача на весь аккаунт. Пополнения нет — оно в панели |
| `list_apps`, `get_app` | чтение | аппы; у `get_app` — адрес, домены, последняя сборка |
| `create_app` | запись | апп из git или Docker-образа; идемпотентен по имени |
| `deploy_files` | запись | выкладка САЙТА (статика, SSR) из файлов разговора: пакует `tar.gz` → `/v1 …/deployments`; создаёт апп при первом вызове, повтор с тем же именем — тот же апп. ⚠ Бэкенд платформа из папки пока не собирает — отказ до загрузки |
| `deploy_app` | запись | новая сборка git/docker-аппа; у docker-аппа с `image` — сначала переключает его на новый образ (`PATCH docker_image`), потом собирает |
| `get_build` | чтение | статус с ожиданием до 45 с; при провале — хвост лога |
| `get_build_logs`, `list_builds` | чтение | лог и история сборок |
| `list_env`, `set_env` | чтение/запись | переменные; значения секретов не отдаются никогда |
| `delete_env`, `remove_domain` | удаление | помечены `destructiveHint` — клиент спросит человека |
| `list_domains`, `add_domain` | чтение/запись | домены и инструкция по DNS |

Удаления аппов нет намеренно: необратимые действия появятся только с
двухшаговым подтверждением.

## Правила, на которых держится

- **Секрет не попадает в контекст модели.** Контекст логируется у провайдера
  модели; секрет, попавший туда, считается утёкшим. `.env` из `deploy_files` в
  архив не кладётся (как у `tatnet deploy`), значения секретов не возвращаются.
- **Пустой список ≠ «ничего нет».** `/v1` отвечает 200 и пустым списком, когда
  ключу не видно ресурсов; инструменты говорят это прямо, если ключ ограничен.
- **Отказ — текст, по которому можно действовать**: что случилось и что делать
  (403 → «расширьте ключ, не повторяйте», 5xx → «наша сторона, повторите»).
- **Лог сборки — недоверенные данные.** Его пишет код клиента; инструкции
  сервера и схемы полей говорят модели не исполнять найденное в нём.
- **«Сборка прошла» ≠ «работает».** `get_build` ждёт конца раскатки и отдаёт
  `deploy_state`; живым апп называется только при `live`.
- **Повторы не плодят ресурсы.** Модели повторяют вызовы; `create_app` и
  `deploy_files` находят апп по имени вместо создания второго.

## Подключение

Адрес сервера — `https://mcp.tatnet.cloud/mcp`.

**Через OAuth** — в клиенте с поддержкой удалённых MCP-серверов (веб-чаты,
десктопные приложения, IDE) достаточно указать адрес и выбрать OAuth: клиент
зарегистрируется сам и откроет вход в TatNet.

**Ключом** — для клиентов, которые передают заголовок, и для скриптов. Ключ
создаётся в панели TatNet → «API-ключи»; можно ограничить проектами и
действиями.

```bash
claude mcp add --transport http tatnet https://mcp.tatnet.cloud/mcp \
  --header "Authorization: Bearer tn_live_…"
```

## Окружение

| Переменная | По умолчанию | |
|---|---|---|
| `LISTEN` | `:8080` | |
| `API_BASE_URL` | `https://api.tatnet.ru/v1` | stage — `…stage…/v1` |
| `PUBLIC_URL` | `https://mcp.tatnet.cloud` | основной внешний адрес (метаданные OAuth) |
| `EXTRA_PUBLIC_URLS` | — | дополнительные внешние имена через запятую (например, на время переезда); на каждом имени сервер говорит от его лица |
| `METRICS_TOKEN` | — | без него `/metrics` отвечает 404 |

Метрики: `tatnet_mcp_tool_calls_total{tool,outcome}`,
`tatnet_mcp_tool_duration_seconds{tool}`, `tatnet_mcp_auth_failures_total{reason}`.

## Разработка

```bash
go build ./... && go test ./...
```

Сквозной тест (`cmd/tatnet-mcp/e2e_test.go`) поднимает настоящий MCP-клиент
SDK против сервера, а сервер — против поддельного `/v1` с состоянием в памяти,
и проверяет последствия вызовов: что ушло в архив, сколько аппов создано,
что попало в ответ модели.

## Каталоги

Сервер один; каталогов, из которых о нём узнают клиенты, три, и в каждый он
подаётся отдельно.

| Каталог | Что нужно | Статус |
|---|---|---|
| [Официальный реестр MCP](https://registry.modelcontextprotocol.io) | `server.json` в корне + владение доменом `tatnet.cloud` | опубликована версия `1.1.1` (2026-10-05) |
| Каталог коннекторов Claude (`claude.ai/directory/manage`) | форма, тестовый аккаунт, документация, политика конфиденциальности | — |
| Плагины ChatGPT (`platform.openai.com/plugins`) | верификация издателя, файл `/.well-known/openai-apps-challenge`, 5+3 тестовых сценария | — |

**Реестр.** Пространство имён `cloud.tatnet/*` подтверждается TXT-записью на
АПЕКСЕ `tatnet.cloud` (не под селектором вроде `_mcp.`), адрес сервера держим
на том же домене (тест `TestServerJSONFitsTheRegistry`; реестр этого не
требует, это наше правило). Ключ Ed25519 — только OpenSSL 3 (системный
`openssl` macOS — LibreSSL, Ed25519 не умеет):

```
O=/opt/homebrew/opt/openssl@3/bin/openssl
$O genpkey -algorithm Ed25519 -out key.pem          # хранить как секрет, не в репозитории
echo "tatnet.cloud. IN TXT \"v=MCPv1; k=ed25519; p=$($O pkey -in key.pem -pubout -outform DER | tail -c 32 | base64)\""
# ↑ эту запись — в зону tatnet.cloud, дождаться распространения
mcp-publisher login dns --domain tatnet.cloud \
  --private-key "$($O pkey -in key.pem -noout -text | grep -A3 priv: | tail -n +2 | tr -d ' :\n')"
mcp-publisher publish
```

Ключ текущей DNS-авторизации хранится локально в конфигурации издателя,
вне этого репозитория. При повторном входе используйте существующий ключ;
его содержимое не должно попадать в README или Git.

⚠ Каждая публикация требует НОВОЙ `version` в `server.json`: реестр
отвергает повтор уже опубликованной версии.

**Аннотации.** Оба каталога читают `title`, `readOnlyHint`,
`destructiveHint`, `openWorldHint` с провода, а ChatGPT отклоняет инструмент,
у которого флаг не указан явно. Тест `TestToolAnnotationsAreExplicitOnTheWire`
проверяет сырой `tools/list`.

## Finding apps and confirming a deployment

`list_apps` accepts exact `repo`, `branch`, `name`, and `domain` filters.
Omitting `project_id` searches all projects visible to the connection.
`limit` defaults to 100 matches (maximum 200). If `next_cursor` is present,
continue with that cursor and the same filters; an empty page can still have
more pages. `scope` states which projects were searched. Filtering happens
across API pages, so later matches remain discoverable.

`get_build` accepts either `build_id` or an exact full `commit_sha`.
Both searches continue through build history, rather than just the latest
50 builds. Omitting both selects the latest build and pins its ID during
that call's polling. When continuing across calls, pass the returned build
ID to keep observing the same build.

`build_succeeded` describes artifact creation. `deployment_succeeded` and
`succeeded` are true only for `status=success` and `deploy_state=live`.
A missing, unknown, rolling or unverified deployment state remains pending;
startup failures terminate with `succeeded=false` and diagnostic log lines.
`finished` means waiting has ended, including failure. Never treat it alone
as a successful deployment.
