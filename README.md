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
Additional tools cover managed Kafka, VM creation and backup policies, regional
S3 discovery and bucket creation, VPC display-name changes and the public AI
model catalog. `list_vm_plans` discovers VM plan UUIDs, specifications and prices.
Use the CLI/API for discovery of VM images/clusters, other
VM operations, databases, DNS zones, S3 credentials and object operations.
App deletion and balance top-ups are unavailable through MCP.

- [MCP overview](https://tatnet.ru/mcp) and [connection docs](https://docs.tatnet.ru/docs/mcp).
- [Publish from Claude Code](https://docs.tatnet.ru/docs/guides/en/claude-code-deploy).
- [Diagnose a failed build](https://docs.tatnet.ru/docs/guides/en/debug-build).
- [CLI deployment from CI](https://docs.tatnet.ru/docs/guides/en/cli-ci-deploy).

## Устройство

- **Тонкий слой над публичным `/v1`.** Своей БД, своих прав и бизнес-логики нет:
  инструменты управления зовут `api.tatnet.ru/v1` через SDK `tatnet-go` или
  общий JSON-транспорт с той же авторизацией вызывающего. `ai_models` отдельно
  читает публичный каталог ИИ без передачи ключа. Права — политика ключа ∩ живая роль создателя, как у CLI и SDK.
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

## Инструменты

| Инструмент | Тип | Что делает |
|---|---|---|
| `whoami` | чтение | аккаунт и проекты, ограничен ли ключ |
| `get_balance` | чтение | баланс: доступно сейчас, деньги, бонусы и открыты ли они. Только владельцу: ключу нужно `billing:read`, OAuth-подключению — выдача на весь аккаунт. Пополнения нет — оно в панели |
| `list_vm_plans` | чтение | каталог тарифов через `/v1/vms/plans`: UUID для `vm_plan_id`, CPU, память в МиБ, диски, семейство CPU и цены в рублях. `cluster_id` фильтрует совместимое оборудование, `role` — назначение (по умолчанию `vm`). Требуется `vm:read` |
| `list_apps`, `get_app` | чтение | аппы; у `get_app` — адрес, домены, последняя сборка |
| `create_app` | запись | апп из git или Docker-образа; идемпотентен по имени |
| `set_app_branch` | запись | смена ветки Git-приложения для будущих сборок и автодеплоя; требует `app:update`, текущую версию не выкатывает — затем вызвать `deploy_app` |
| `deploy_files` | запись | выкладка САЙТА (статика, SSR) из файлов разговора: пакует `tar.gz` → `/v1 …/deployments`; создаёт апп при первом вызове, повтор с тем же именем — тот же апп. ⚠ Бэкенд платформа из папки пока не собирает — отказ до загрузки |
| `deploy_app` | запись | новая сборка git/docker-аппа; у docker-аппа с `image` — сначала переключает его на новый образ (`PATCH docker_image`), потом собирает |
| `get_build` | чтение | статус с ожиданием до 45 с; при провале — хвост лога |
| `get_build_logs`, `list_builds` | чтение | лог и история сборок |
| `list_env`, `set_env` | чтение/запись | переменные; значения секретов не отдаются никогда |
| `delete_env`, `remove_domain` | удаление | помечены `destructiveHint`; подтверждение зависит от клиента |
| `list_domains`, `add_domain` | чтение/запись | домены и инструкция по DNS |
| `kafka_regions`, `kafka_list_clusters`, `kafka_get_cluster` | чтение | Kafka Dev/HA, ставки часа, управление и прогноз за 730 часов |
| `kafka_create_cluster` | запись | выделенный платный Kafka-кластер; пароль выдаётся один раз |
| `kafka_resume_cluster` | запись | оплатить час остановленных подписок и запустить узлы; оплаченные подписки не списываются повторно |
| `kafka_update_allowlist`, `kafka_delete_cluster` | изменение/удаление | публичный доступ или удаление кластера вместе с данными |
| `kafka_canary_node_power` | изменение | закрытая проверка start/stop узла для настроенного canary-аккаунта; VM продолжает тарифицироваться |
| `ai_models` | чтение | публичный каталог chat/image/video; без генерации и расходов |
| `create_load_balancer`, `get_load_balancer` | запись/чтение | создать платный LB и проверить статус; `floating_ip_id` закрепляет свободный IP аккаунта, сохраняющийся после удаления LB |
| `rename_vpc` | запись | изменить отображаемое имя VPC |
| `enable_nat_gateway` | изменение | включить NAT для VPC: `fip_id` выбирает свободный адрес аккаунта в том же регионе; без него выделяется новый. Адрес и NAT тарифицируются; подключение асинхронное |
| `disable_nat_gateway` | изменение | выключить NAT-шлюз VPC; `retain_ip=true` сохраняет IP за аккаунтом с продолжением тарификации. Снятие NAT асинхронно; перед повторным использованием адреса нужно дождаться `available` через API/CLI. `expected_fip_id` защищает от выключения заменённого шлюза |
| `list_s3_regions`, `get_s3_connection_info` | чтение | регионы, endpoints и примеры подключения S3 |
| `create_s3_bucket` | запись | создать бакет, приватный по умолчанию |
| `create_vm` | запись | создать платную VM, при необходимости сразу с политикой автобэкапов |
| `list_vm_backups`, `get_vm_backup_policy` | чтение | точки восстановления, размеры, сроки защиты и расписание |
| `set_vm_backup_policy` | запись | расписание и срок хранения будущих автоматических копий |
| `delete_vm_backup` | удаление | безвозвратно удалить точку восстановления с проверками API |

Kafka тарифицируется в API: почасовые VM + 30% за управление; IPv4 и
дополнительные диски без наценки. Управление оплачивается после первого `ready`.
MCP передаёт цены и решения биллинга, не рассчитывает их самостоятельно.

Удаление аппов не поддерживается. Удаление Kafka и бэкапов уже доступно:
инструменты помечены `destructiveHint`; показ подтверждения зависит от клиента.
Серверного двухшагового подтверждения нет, права и защитные ограничения проверяет API.

## Правила, на которых держится

- **Ключи авторизации не возвращаются инструментами.** `.env` из `deploy_files`
  в архив не кладётся (как у `tatnet deploy`), значения секретных env не читаются.
  Исключение — создание Kafka: начальный пароль возвращается один раз и попадёт
  в контекст клиента. Сохраните его в менеджере секретов; не публикуйте логи вызова.
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
| `LISTEN` | `:$PORT`, иначе `:8080` | явный `LISTEN` имеет приоритет над портом Apps Platform |
| `PORT` | — | порт, выданный Apps Platform; используется, когда `LISTEN` не задан |
| `API_BASE_URL` | `https://api.tatnet.ru/v1` | stage — `…stage…/v1` |
| `PUBLIC_URL` | `https://mcp.tatnet.cloud` | основной внешний адрес (метаданные OAuth) |
| `EXTRA_PUBLIC_URLS` | — | дополнительные внешние имена через запятую (например, на время переезда); на каждом имени сервер говорит от его лица |
| `METRICS_TOKEN` | — | без него `/metrics` отвечает 404 |
| `OIDC_ISSUER` | — | издатель OAuth; без него принимаются только API-ключи |
| `OIDC_JWKS_URL` | `$OIDC_ISSUER/.well-known/jwks.json` | адрес ключей подписи; можно указать внутренний адрес |
| `MCP_INTERNAL_SECRET` | — | обязателен при включённом OAuth; совпадает с `V1_MCP_INTERNAL_SECRET` у API, хранится как секрет |

Метрики: `tatnet_mcp_tool_calls_total{tool,outcome}`,
`tatnet_mcp_tool_duration_seconds{tool}`, `tatnet_mcp_auth_failures_total{reason}`.

## Разработка

```bash
go vet ./...
go build ./...
go test -count=1 ./...
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

## Каталог моделей ИИ

`ai_models(kind="chat" | "text" | "image" | "video")` читает публичный каталог
`ai.tatnet.cloud`: модели, умения и клиентские цены. Инструмент не передаёт
ключ управления облаком в шлюз инференса и не запускает генерацию.

Платные операции генерации ИИ требуют отдельного `tnai_live_…`, тогда как текущий
OAuth/ключ MCP авторизует управление облаком. Без безопасного обмена на
ключ инференса конкретного аккаунта генерацию через remote MCP не добавляем.
Оставшийся шаг — такой обмен/делегирование с проверкой прав аккаунта;
передавать секрет через аргументы модели или общий ключ сервера не следует.
Для генерации сейчас используйте `tatnet ai` с собственным ключом инференса.

## Rename a VPC

`rename_vpc` accepts `vpc_id` and `name`. It changes only the display label through `PATCH /v1/vpcs/{id}` and requires `vpc:update`; subnet, ports, NAT and OVN identifiers stay unchanged.

## S3

`list_s3_regions` показывает регионы и публичные endpoints;
`get_s3_connection_info` возвращает настройки подключения выбранного региона;
`create_s3_bucket` создаёт приватный по умолчанию бакет с необязательным `region`.
При отсутствии региона API использует регион по умолчанию. Авторизация — та же
политика `/v1`, включая OAuth-подключения; права на бакеты проверяет API.

## VM и бэкапы

VM tools: `create_vm` accepts the public VM creation settings, including an optional atomic `backup_policy`; `list_vm_backups` returns recovery point origin, expiry, compressed size and immutable deletion deadlines. API permissions and billing checks remain authoritative.

VM backups: `delete_vm_backup` deletes a selected point through the public API, preserving active-job, retention and chain-dependency guards. Abandoned legacy placeholders can be explicitly deleted after seven days without a stored result or dispatch.

`get_vm_backup_policy` reads the schedule; `set_vm_backup_policy` accepts
`project_id`, `vm_id`, `enabled`, `hour_utc` (0–23 UTC) and `retention_days`
(1–365). Storage costs 6 RUB per compressed decimal GB-month (730 hours),
separate from the VM subscription, without prepaid slots. Disabling a policy
preserves copies and storage charges. Manual copies do not expire; old locks
and chain dependencies remain enforced. New captures are independent full
compressed points. See the [user guide](https://docs.tatnet.ru/docs/infrastructure/vm-backups).

## Выпуск и проверка версии

Выпуск `1.5.0` добавляет `enable_nat_gateway` и `disable_nat_gateway` с сохранением
публичного IP. Для повторного подключения дождитесь `available` и передайте
`fip_id`; сохранённый адрес продолжает тарифицироваться.

Тег Git, GitHub Release, образ в GHCR, работающий сервер и карточка MCP Registry
обновляются отдельно. `server.json` описывает публикацию в Registry; он не
переключает образ приложения и не задаёт версию бинарника. Версия в MCP
`initialize` приходит из Docker build-аргумента `VERSION` (в CI — ref + SHA).

На 2026-10-22 проверено:

| Где | Состояние |
|---|---|
| Git | последний тег `v1.4.3`, SHA `3743a1a40edf65914df972e021e8890c13d0e9a3` |
| GitHub Releases | последний оформленный выпуск `v1.1.0`; теги `v1.2.0`–`v1.4.3` существуют отдельно |
| Production | образ по SHA `v1.4.3`, две реплики, `deploy_state=live`, `/healthz` отвечает 200 |
| MCP Registry | опубликована `1.1.1`; в исходниках на момент проверки — `1.4.3`; этот PR готовит `1.4.4` |
| `main` после `v1.4.3` | до этой проверки только README о тарификации бэкапов |

Для уже работающей `v1.4.3` можно оформить GitHub Release на существующем теге
и опубликовать соответствующий `server.json` в Registry. Пересоздавать тег и
перекатывать тот же код не требуется. Исправления из этой проверки требуют
следующей patch-версии (`1.4.4`) после слияния и проверки CI.

Порядок нового выпуска:

1. Обновить `server.json` до ещё не опубликованной версии, описать изменения,
   запустить vet/build/tests и дождаться успешного CI на коммите выпуска.
2. Поставить `v<version>` на проверенный коммит. Workflow `image` публикует
   образы на push в `main` и на тег. Только `main` пишет тег полного SHA;
   релизный push пишет `v<version>` и не перезаписывает SHA-образ. Сборка образа и тесты — отдельные workflows:
   наличие образа само по себе не доказывает, что тесты прошли.
3. Оформить GitHub Release с изменениями, результатами проверок и полным SHA.
4. При изменении сервера выбрать образ `ghcr.io/tatnet-ru/tatnet-mcp:<полный-SHA>`
   через `tatnet app update <app-id> --image <image>`, затем
   `tatnet app deploy <app-id>`. Указать нужный проект CLI (`-p <project-id>`).
5. Через `tatnet app get <app-id> -o json` проверить `deploy_state=live`,
   `running_docker_image` и `running_docker_image_digest`; проверить `/healthz`
   и авторизованный MCP `initialize`/`tools/list`. При необходимости отката
   вернуть предыдущий SHA образа и повторить deploy.
6. Из checkout того же тега выполнить `mcp-publisher publish` существующей
   учётной записью издателя. Проверить `server.version` и `isLatest` по
   [Registry API](https://registry.modelcontextprotocol.io/v0.1/servers/cloud.tatnet%2Fmcp/versions/latest).
   `gh release create` и пуш тега не публикуют карточку Registry.

Полнота поддержки операций определяется `tools/list`. Описание карточки Registry
краткое и не заменяет таблицу инструментов выше.

## Несколько VPC на ВМ

Каждая VPC подключается отдельным сетевым интерфейсом в том же аккаунте и регионе.
Перед изменением интерфейсов ВМ нужно выключить; настройки применяются при запуске.
Основной интерфейс и маршрут по умолчанию защищены от отключения.

`attach_vm_interface` принимает `project_id`, `vm_id`, `vpc_id`;
`detach_vm_interface` — `project_id`, `vm_id`, `interface_id`. Обе операции
используют общий публичный API с проверками `vm:update` и `vpc:write`.

### Managed PostgreSQL

- `get_postgres_cluster` reads the current cluster, dedicated reader endpoint,
  replica node states, byte lag and pending changes.
- `set_postgres_read_replicas` sets `count` (0–5) for a `project_id` and
  `cluster_id`. These are additional paid nodes on the cluster plan, independent
  of HA. Count zero removes readers; the primary database remains. Replicas
  never automatically promote and can lag. The read endpoint shares the writer
  address on TCP/5433. After changing intent, inspect `get_postgres_cluster`
  until `read_replicas_pending` is false and verify the observed node states.

Both tools use the common public API and preserve API-key/OAuth authorization.
