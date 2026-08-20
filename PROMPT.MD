# MASTER PROMPT V2 — Второй этап глубокого улучшения Email & Communication Platform + интеграция Stalwart Mail Server

Продолжай работу **с текущего состояния проекта после уже выполненного редизайна и функционального усиления**.

НЕ откатывай предыдущие изменения. НЕ создавай проект заново. НЕ заменяй уже хорошо работающие компоненты только ради переписывания.

Предыдущий этап уже реализовал:

* Silver / Graphite design system;
* новый Login;
* единую тему;
* Mail Workspace;
* Inbox split-view;
* Reading Pane;
* Compose window;
* Contacts;
* Departments;
* Tasks & Reminders;
* Command Palette;
* Admin Dashboard;
* Users;
* Logs & Audit;
* Security pages;
* Broadcast;
* базовые health-индикаторы;
* USER / ADMIN разделение;
* backend-доработки;
* UI/E2E tests.

Теперь задача — провести **ВТОРОЙ, более глубокий проход по всему продукту**.

На этом этапе необходимо превратить приложение из хорошего корпоративного webmail/admin-интерфейса в **настоящую Email Infrastructure + Business Mail Platform**, приблизив реализацию к архитектуре ТЗ v3.0.

Ключевое изменение этого этапа:

> **Начать полноценную интеграцию Stalwart Mail Server как mail-core системы.**

Stalwart должен быть не декоративным пунктом в UI и не mock-интеграцией, а реально подключенным инфраструктурным компонентом.

Основная архитектурная идея:

**Наш Go backend / Control Plane управляет продуктовой логикой, пользователями, организациями, политиками, аудитом и интеграциями. Stalwart отвечает за mail-core функции, где это архитектурно оправдано.**

Исходное ТЗ предусматривает Stalwart и/или Postfix как SMTP/Mail Core, а для будущего Business Mail — Stalwart с IMAP/JMAP. Также в ТЗ отдельно заложены SMTP Relay, Inbound Email, domains/DNS, DKIM, security pipeline, deliverability и дальнейший Business Mail. Используй ТЗ как главный источник требований.

---

# 0. ГЛАВНОЕ ПРАВИЛО

Не делай superficial redesign.

На втором этапе необходимо улучшить:

**Architecture + Backend + Mail Core + Infrastructure + UX + Admin + Observability + Security + Product Depth.**

Каждое изменение должно либо:

1. улучшать реальную функциональность;
2. уменьшать количество действий пользователя;
3. улучшать наблюдаемость системы;
4. повышать безопасность;
5. приближать проект к production;
6. устранять технический долг;
7. улучшать соответствие ТЗ.

Не добавляй fake functionality.

Если кнопка существует — она должна работать.

Если status отображается — он должен приходить из реального источника.

Если Stalwart недоступен — UI должен показывать реальное состояние `Unavailable/Degraded`, а не фиктивный `Operational`.

---

# 1. СНАЧАЛА ПОЛНЫЙ RE-AUDIT

Перед написанием нового кода повторно изучи:

* всё ТЗ v3.0;
* текущий repository;
* frontend;
* Go API;
* worker;
* PostgreSQL schema;
* Redis;
* NATS;
* MinIO;
* authentication;
* users;
* departments;
* mail storage;
* current SMTP implementation;
* message delivery;
* attachments;
* audit;
* security;
* Docker;
* compose files;
* environment variables;
* migrations;
* tests.

Отдельно изучи все изменения предыдущего этапа.

Не предполагай, что отчет предыдущего агента полностью соответствует реальному состоянию repository.

**Проверь каждое утверждение непосредственно по коду.**

---

# 2. GAP ANALYSIS ПРОТИВ ТЗ

До реализации составь внутреннюю матрицу:

| Module | Requirement | Current state | Missing | Priority |
| ------ | ----------- | ------------- | ------- | -------- |

Проверь минимум:

* Authentication
* Users
* Organizations
* Projects
* Departments
* Domains
* DNS
* API Keys
* SMTP Credentials
* Email API
* SMTP Relay
* Outbound delivery
* Inbound delivery
* Mailboxes
* Attachments
* DKIM
* SPF
* DMARC
* Webhooks
* Events
* NATS
* Redis
* MinIO
* Stalwart
* Security
* Quarantine
* Audit
* Deliverability
* IP Pools
* Reputation
* Abuse protection
* Logs
* Monitoring
* Self-Hosted
* Backup
* Health
* Admin
* Webmail.

После этого реализуй максимум P0/P1-функций, которые можно корректно добавить в текущую архитектуру.

---

# 3. STALWART — ГЛАВНЫЙ P0 ЭТОГО ЭТАПА

Добавить **Stalwart Mail Server** в инфраструктуру проекта.

Не ограничиваться запуском контейнера.

Нужна полноценная интеграция:

**Application → Control Plane → Stalwart → Mailbox/SMTP/JMAP/IMAP**

с четко определенными boundaries.

Stalwart должен постепенно стать mail-core для:

* corporate mailboxes;
* inbound mail;
* authenticated submission;
* mailbox storage;
* IMAP;
* JMAP;
* SMTP;
* aliases/groups, если поддерживаются выбранной конфигурацией;
* domain/mailbox provisioning.

При этом существующий Go backend остается владельцем продуктовой логики и Control Plane.

---

# 4. НЕ ПРИВЯЗЫВАТЬ БИЗНЕС-ЛОГИКУ К STALWART

Создай abstraction layer.

Например:

`MailCoreProvider`

или архитектурно эквивалентный интерфейс.

Он должен абстрагировать операции типа:

* create domain;
* update domain;
* delete domain;
* create mailbox;
* update mailbox;
* disable mailbox;
* create alias;
* mailbox quota;
* get mailbox status;
* provision account;
* delivery status;
* health.

Конкретная реализация:

`StalwartProvider`

Нельзя размазывать Stalwart-specific HTTP/API/config calls по десяткам Go handlers.

---

# 5. STALWART INFRASTRUCTURE

Добавить Stalwart в local/self-hosted deployment.

Обновить:

* Docker Compose;
* `.env.example`;
* infrastructure configuration;
* health checks;
* volumes;
* networking;
* secrets;
* documentation.

Stalwart должен иметь persistent storage.

Не хранить production secrets непосредственно в repository.

Добавить environment/configuration параметры вроде:

* Stalwart URL;
* internal endpoint;
* credentials/token;
* public mail hostname;
* SMTP hostname;
* IMAP hostname;
* JMAP endpoint;
* storage paths;
* TLS configuration.

Названия адаптируй под существующий configuration style проекта.

---

# 6. STALWART HEALTH

Интегрировать Stalwart в существующий health system.

Admin → System Health должен показывать реальные состояния:

* API;
* PostgreSQL;
* Redis;
* NATS;
* MinIO;
* **Stalwart**;
* Worker.

Для Stalwart:

* availability;
* response latency;
* version, если возможно получить безопасно;
* SMTP status;
* JMAP status;
* IMAP status, если можно корректно проверить;
* last successful health check.

Не выполнять тяжелые проверки на каждом frontend request.

Использовать backend health aggregation/cache.

---

# 7. DOMAIN PROVISIONING

Связать Domains приложения со Stalwart.

Когда ADMIN создает/активирует корпоративный mail domain, backend должен иметь четкий provisioning lifecycle.

Например:

`CREATED`

→ `DNS_PENDING`

→ `VERIFIED`

→ `PROVISIONING`

→ `ACTIVE`

или:

→ `FAILED`

Не считай domain активным только потому, что строка появилась в PostgreSQL.

---

# 8. DOMAIN CENTER

Создать или серьезно улучшить страницу:

**Admin → Domains**

Она должна быть production-level.

Показывать:

* domain;
* organization;
* status;
* verification;
* SPF;
* DKIM;
* DMARC;
* MX;
* mailboxes;
* aliases;
* created;
* last check.

Filters:

* Active;
* Pending;
* Failed;
* DNS issues.

При клике открывать Domain Detail.

---

# 9. DOMAIN DETAIL

Domain Detail сделать полноценным operational workspace.

Tabs:

### Overview

* domain status;
* organization;
* mailboxes;
* sending status;
* inbound status;
* health.

### DNS

Показывать необходимые DNS records:

* MX;
* SPF;
* DKIM;
* DMARC;
* Return-Path, если используется;
* MTA-STS/TLS-RPT, если реализованы.

Для каждой записи:

* type;
* host;
* expected value;
* detected value;
* status;
* last checked;
* Copy.

### Mailboxes

* accounts;
* quota;
* usage;
* status.

### Aliases

Если реализовано.

### Security

* recent quarantine;
* suspicious activity;
* policy.

### Activity

* provisioning;
* DNS checks;
* configuration changes;
* admin audit.

---

# 10. DNS VERIFICATION

Добавить реальную DNS verification архитектуру.

Не fake-check.

Backend должен уметь проверять необходимые DNS records.

Проверять:

* MX;
* SPF;
* DKIM;
* DMARC.

По возможности:

* MTA-STS;
* TLS-RPT.

UI должен четко различать:

* Verified;
* Missing;
* Invalid;
* Pending;
* Warning.

Добавить:

**Recheck DNS**

и хранить timestamp последней проверки.

---

# 11. DKIM

Реализовать нормальный DKIM lifecycle совместно с mail-core.

Не показывать private key пользователю.

UI должен показывать:

* selector;
* DNS record;
* verification;
* created;
* rotation status.

Архитектуру подготовить к rotation.

Секретный key material не должен попадать:

* в frontend;
* в audit metadata;
* в application logs.

---

# 12. MAILBOXES

Добавить полноценное управление корпоративными mailbox.

ADMIN должен иметь:

**Mail → Mailboxes**

или аналогичный раздел.

Mailbox:

* display name;
* local part;
* domain;
* full email;
* user;
* department;
* status;
* quota;
* usage;
* last activity;
* created.

Actions:

* Create;
* Edit;
* Disable;
* Enable;
* Change quota;
* Reset credentials/password, если архитектура это позволяет;
* View activity.

---

# 13. USER ↔ MAILBOX

Четко разделить:

**Application User**

и

**Mailbox Account**.

Они могут быть связаны 1:1 сейчас, но архитектура не должна жестко предполагать это навсегда.

Подготовить модель для будущих:

* aliases;
* shared mailboxes;
* service mailboxes;
* groups.

---

# 14. MAILBOX CREATION

При создании mailbox:

1. Validate domain.
2. Validate local part.
3. Check uniqueness.
4. Create DB provisioning record.
5. Provision in Stalwart.
6. Confirm result.
7. Update status.
8. Audit operation.

При ошибке:

* не оставлять систему в непонятном состоянии;
* сохранить provisioning error;
* позволить Retry.

---

# 15. MAILBOX DETAIL

Сделать detail drawer/page:

* email;
* owner;
* department;
* domain;
* quota;
* usage;
* state;
* aliases;
* last login;
* last mail activity;
* recent audit.

Admin actions должны быть contextual.

---

# 16. JMAP

Поскольку Stalwart планируется использовать как mail-core для Business Mail, исследуй и подключи **JMAP** там, где это является наиболее правильным способом работы Webmail.

Не строй второй собственный mailbox protocol поверх REST без необходимости.

Определи, какие операции frontend/backend разумно выполнять через:

* текущий Go API;
* Stalwart JMAP;
* internal Stalwart integration.

Security boundaries должны оставаться четкими.

Нельзя передавать административные Stalwart credentials браузеру.

---

# 17. IMAP

Подготовить и проверить IMAP support.

В UI пользователя можно добавить:

**Settings → Mail Clients**

Показывать параметры подключения:

* IMAP server;
* port;
* encryption;
* SMTP server;
* submission port;
* username.

Пароль не показывать.

Если поддерживаются application passwords — реализовать только при наличии корректной security architecture.

---

# 18. SMTP SUBMISSION

Настроить Stalwart SMTP submission согласно архитектуре проекта.

Основной пользовательский submission:

* 587 + STARTTLS.

Если требуется:

* 465 implicit TLS.

Не использовать пользовательский submission через TCP/25.

TCP/25 — server-to-server SMTP.

Это должно соответствовать исходной архитектуре ТЗ.

---

# 19. OUTBOUND DELIVERY

Проверь существующий flow отправки.

Compose должен действительно проходить через production-like mail flow.

Нужно добиться понятной цепочки:

**Compose**

→ application authorization

→ validation

→ policy

→ mail-core/submission

→ queue

→ delivery

→ event

→ UI status.

Не допускай ситуацию, когда UI показывает `Sent`, хотя письмо фактически только записалось в БД.

---

# 20. DELIVERY STATES

Расширить реальные статусы письма:

* Draft
* Queued
* Processing
* Sent
* Delivered
* Deferred
* Bounced
* Failed
* Quarantined

Не использовать статусы, для которых backend не имеет сигнала.

В Reading Pane для отправленных писем добавить delivery status.

---

# 21. MESSAGE EVENT TIMELINE

Для ADMIN и, в сокращенном виде, USER добавить timeline письма.

Например:

`14:32:01 Accepted`

`14:32:02 Security scan passed`

`14:32:02 Queued`

`14:32:03 SMTP attempt`

`14:32:04 Sent`

`14:32:07 Delivered`

или:

`Deferred — 451 temporary rate limit`

Timeline должен строиться из реальных событий.

---

# 22. MESSAGE TRACE

Создать ADMIN:

**Message Trace**

Это должен быть один из ключевых operational tools.

Поиск по:

* Message ID;
* sender;
* recipient;
* subject;
* date;
* status;
* domain.

Результат:

* accepted;
* queue;
* route;
* delivery attempts;
* SMTP responses;
* final status;
* security decision.

---

# 23. SELF-EMAIL BUG

В предыдущем этапе обнаружена проблема:

> письмо самому себе не попадает во Inbox из-за `ON CONFLICT DO NOTHING`, поскольку Sent copy занимает конфликтующую пару mailbox/message.

Теперь это необходимо **архитектурно исправить**.

Не делать костыль в UI.

Проверь schema и модель mailbox-message relation.

Sent и Inbox должны иметь возможность ссылаться на один message через разные mailbox/folder relations.

Добавь migration, если требуется.

Обязательно regression test:

**[user@example.com](mailto:user@example.com) → [user@example.com](mailto:user@example.com)**

После отправки:

* письмо есть в Sent;
* письмо есть в Inbox;
* не создаются неправильные duplicate message bodies;
* attachments работают;
* thread работает.

---

# 24. THREADING

Улучшить email threading.

Использовать реальные mail headers, где доступны:

* Message-ID;
* In-Reply-To;
* References.

Не группировать письма исключительно по одинаковому subject.

Conversation view должен правильно отображать:

* original;
* replies;
* forwarded context;
* timestamps;
* participants.

---

# 25. COMPOSE V2

Compose уже реализован — теперь доведи его до более высокого уровня.

Добавить:

* To;
* Cc;
* Bcc;
* department recipients;
* attachments;
* drag & drop;
* attachment size validation;
* autosave indicator;
* sending state;
* failure recovery;
* retry;
* draft state.

При отправке:

`Sending…`

→ `Sent`

или:

`Failed to send — Retry`

Не закрывать compose бесследно при сетевой ошибке.

---

# 26. SCHEDULE SEND

Если после интеграции backend архитектура позволяет сделать это корректно — добавить Schedule Send.

Options:

* Later today;
* Tomorrow morning;
* Custom date/time.

Scheduled mail должен храниться server-side.

Нельзя реализовывать schedule через `setTimeout()` в браузере.

Worker должен отправлять письмо независимо от открытого браузера.

---

# 27. DRAFTS

Улучшить drafts.

Draft должен сохранять:

* recipients;
* CC;
* BCC;
* subject;
* body;
* attachments;
* updated_at.

Показывать:

`Saved 12 sec ago`

и состояние:

`Saving…`

При конфликте нескольких вкладок не терять более свежие данные без предупреждения.

---

# 28. ATTACHMENTS

Провести отдельный аудит attachments.

Проверить:

* upload;
* download;
* permissions;
* filename;
* MIME;
* size;
* storage;
* deletion;
* draft attachments;
* sent attachments;
* inbound attachments.

Использовать MinIO/S3 согласно существующей архитектуре.

Не отдавать object storage bucket напрямую публичным.

---

# 29. SECURITY PIPELINE

Начать связывать mail flow с Security Center.

Для каждого inbound/outbound message подготовить/использовать security verdict.

Минимум:

* spam;
* phishing;
* malware;
* spoofing;
* suspicious;
* clean.

Если Rspamd/ClamAV уже существуют — подключить реальные данные.

Если нет — подготовить архитектуру интеграции, но не генерировать фиктивные результаты.

---

# 30. RSPAMD

Если текущая инфраструктура позволяет — добавить/подключить Rspamd.

Проверять реальные mail signals.

Результаты должны быть доступны backend.

Security detail:

* score;
* action;
* symbols/signals;
* SPF;
* DKIM;
* DMARC;
* URLs;
* sender reputation, если существует.

---

# 31. CLAMAV

Подключить ClamAV для attachments/mail scanning, если это возможно в текущем deployment.

При обнаружении malware:

* не отдавать опасное вложение обычному USER;
* отправить письмо в quarantine согласно policy;
* создать security event;
* audit;
* показать ADMIN причину.

---

# 32. QUARANTINE

Сделать полноценный:

**Admin → Security → Quarantine**

Показывать:

* sender;
* recipient;
* subject;
* received;
* risk;
* category;
* signals;
* status.

Actions:

* Release;
* Delete;
* Block sender;
* Block domain;
* Mark safe, если это поддерживает policy.

Перед Release показывать security warning.

---

# 33. SAFE MESSAGE PREVIEW

Для quarantine нельзя автоматически исполнять потенциально опасный HTML.

Использовать безопасный preview.

Не загружать remote resources автоматически.

Не исполнять scripts.

Ссылки и dangerous content должны быть neutralized согласно security architecture.

---

# 34. SECURITY CENTER V2

Переработать Security Center в настоящий SOC-like operational view.

Показывать:

* quarantined;
* phishing;
* malware;
* spam;
* spoofing;
* failed logins;
* blocked events;
* suspicious outbound activity.

Добавить recent incidents.

Не делать 15 гигантских KPI cards.

Использовать компактный operational layout.

---

# 35. SMTP CREDENTIALS

Добавить/улучшить:

**Admin / Developer → SMTP Credentials**

Credentials:

* name;
* username;
* domain/project;
* created;
* last used;
* status.

Secret/password:

* показывать один раз после создания;
* хранить безопасно;
* позволить rotate;
* revoke.

Не логировать secret.

---

# 36. API KEYS

Добавить production-quality API Key management.

Показывать:

* name;
* prefix;
* project;
* domains;
* permissions;
* created;
* last used;
* status.

Actions:

* create;
* rotate;
* revoke.

Full key показывается только один раз.

Audit:

* created;
* rotated;
* revoked.

---

# 37. ORGANIZATIONS / PROJECTS

ТЗ предусматривает hierarchy выше domain.

Проверь текущую модель.

Если сейчас всё построено только вокруг USER/ADMIN и departments — начни аккуратно вводить:

**Organization → Project → Domain**

Не ломая текущую корпоративную UX-модель.

Для single-organization deployment можно скрывать лишнюю сложность от обычного USER.

ADMIN должен иметь полноценное управление.

---

# 38. НЕ ПУТАТЬ DEPARTMENT И ORGANIZATION

Department — внутреннее подразделение компании.

Organization — tenant/business entity.

Они не должны быть одной сущностью.

Пример:

Organization:

`Company A`

Departments:

`Finance`

`HR`

`IT`

`Sales`

Projects:

`Production`

`Marketing`

Domains:

`company.kz`

`notify.company.kz`

---

# 39. ADMIN INFORMATION ARCHITECTURE V2

После добавления infrastructure modules пересмотри sidebar.

Не превращай sidebar в список из 30 элементов.

Сгруппировать.

Например:

### MAIL

* Mail
* Contacts
* Messages
* Reminders

### MANAGEMENT

* Users
* Departments
* Organizations
* Domains

### DEVELOPER

* API Keys
* SMTP Credentials
* Webhooks

### OPERATIONS

* Message Trace
* Delivery
* Logs
* Audit

### SECURITY

* Security Center
* Quarantine

### SYSTEM

* Health
* Infrastructure
* Settings

Используй раскрывающиеся группы только если это реально улучшает навигацию.

---

# 40. ADMIN COMMAND CENTER

Admin Dashboard сделать еще более operational.

Вместо декоративного dashboard дать ответ:

**Что сейчас происходит с системой?**

Показывать:

* mail accepted;
* queued;
* delivered;
* deferred;
* failed;
* quarantine;
* active users;
* failed logins;
* infrastructure health;
* recent incidents.

Добавить блок:

**Needs attention**

Например:

* Domain DNS incomplete;
* Stalwart unavailable;
* queue backlog;
* failed deliveries;
* quarantine spike;
* worker unavailable.

Только реальные события.

---

# 41. INFRASTRUCTURE PAGE

Добавить:

**Admin → System → Infrastructure**

Компоненты:

* Web;
* API;
* PostgreSQL;
* Redis;
* NATS;
* MinIO;
* Worker;
* Stalwart;
* Rspamd;
* ClamAV.

Для каждого:

* status;
* latency;
* version, если безопасно;
* last check;
* optional details.

---

# 42. WEBHOOKS

Реализовать/улучшить Webhook management согласно ТЗ.

Webhook:

* endpoint;
* events;
* status;
* signing;
* created;
* last delivery.

Delivery history:

* event;
* timestamp;
* HTTP status;
* duration;
* attempts.

Actions:

* retry;
* disable;
* rotate secret.

Secret показывать только один раз.

---

# 43. EMAIL EVENTS

Подготовить event model минимум для:

* accepted;
* queued;
* sent;
* delivered;
* deferred;
* bounced;
* failed;
* quarantined.

Использовать versioned events.

Обеспечить idempotency consumers.

NATS JetStream должен использоваться там, где уже соответствует архитектуре проекта.

---

# 44. QUEUE OBSERVABILITY

Admin должен видеть состояние mail queues.

Не обязательно raw NATS UI.

Сделать product-level representation:

* queued;
* processing;
* deferred;
* failed;
* oldest queued;
* throughput.

Добавить фильтры.

Если message stuck — ADMIN должен иметь возможность найти его через Message Trace.

---

# 45. DELIVERY ANALYTICS

Создать/улучшить Deliverability.

Минимум:

* accepted;
* sent;
* delivered;
* deferred;
* bounced;
* failed.

Filters:

* period;
* domain;
* organization;
* project.

Если provider detection существует:

* Gmail;
* Outlook;
* Yahoo;
* Other.

Не придумывать provider metrics без данных.

---

# 46. DOMAIN HEALTH SCORE

Если достаточные реальные данные уже существуют, начать Domain Health.

Не генерировать случайный score.

Score должен рассчитываться из понятных сигналов:

* SPF;
* DKIM;
* DMARC;
* delivery;
* bounce;
* complaint;
* block;
* reputation.

Если данных недостаточно — показывать:

`Not enough data`

вместо fake `92/100`.

---

# 47. OUTBOUND ABUSE

Начать фундамент Outbound Abuse Protection из ТЗ.

Минимум:

* sending velocity;
* failures;
* bounce spikes;
* suspicious recipient patterns;
* credential activity.

Actions архитектурно:

* Allow;
* Throttle;
* Hold;
* Suspend.

Все автоматические действия должны попадать в audit.

---

# 48. RATE LIMITS

Проверь существующие rate limits.

Добавить observability для ADMIN.

Не показывать секретную security configuration обычному USER.

ADMIN должен видеть хотя бы:

* triggered limits;
* entity;
* timestamp;
* action.

---

# 49. NOTIFICATION CENTER V2

Подключить уведомления к реальным operational events.

USER:

* mail;
* message;
* reminder;
* failed send.

ADMIN:

* domain verification failure;
* Stalwart degraded;
* worker unavailable;
* quarantine;
* suspicious login;
* broadcast completion;
* queue problem.

Не спамить одинаковыми уведомлениями.

Добавить deduplication.

---

# 50. USER MAIL EXPERIENCE V2

После инфраструктурных изменений снова пройти USER webmail.

Основной интерфейс должен оставаться простым.

USER не должен видеть:

* NATS;
* DKIM internals;
* Stalwart;
* SMTP queues;
* infrastructure internals.

Для USER продукт остается:

**Mail + Contacts + Messages + Reminders.**

Infrastructure complexity принадлежит ADMIN.

---

# 51. INBOX DENSITY

Еще раз улучшить Inbox.

Цель:

на стандартном laptop viewport пользователь должен видеть достаточное количество писем без ощущения тесноты.

Добавить настройку плотности только если она реально полезна:

* Comfortable;
* Compact.

Не возвращать theme settings.

---

# 52. READING PANE V2

Улучшить визуальную иерархию.

Не показывать все technical metadata постоянно.

Primary:

* subject;
* sender;
* recipients;
* date;
* content;
* attachments;
* actions.

Technical details скрывать под:

**Show details**

Для ADMIN можно показывать расширенный trace.

---

# 53. CONTACTS V2

Добавить более полезную интеграцию с mail.

Contact detail:

* department;
* email;
* recent correspondence;
* last contacted;
* quick compose;
* quick message;
* open related mail.

---

# 54. DEPARTMENTS V2

Department должен стать полноценным organizational filter.

При открытии отдела:

* members;
* unread/recent communication;
* contacts;
* quick compose to department;
* recent activity, если разрешено.

Не раскрывать пользователю чужие приватные mail contents.

---

# 55. INTERNAL MESSENGER

Messenger не является главным P0 этого этапа.

Не трать большую часть времени на него, пока mail-core не стабилен.

Но существующий Messenger:

* не ломать;
* привести к общей design system;
* проверить realtime;
* проверить unread;
* проверить notifications.

---

# 56. AUDIT V2

Расширить audit на новые сущности.

Обязательно audit:

* domain created;
* DNS verification;
* mailbox created;
* mailbox disabled;
* alias changed;
* SMTP credential created;
* credential revoked;
* API key created;
* API key revoked;
* quarantine release;
* security policy action;
* Stalwart provisioning failure;
* admin changes.

Audit не должен содержать секреты.

---

# 57. LOG REDACTION

Провести security audit логирования.

Исключить:

* passwords;
* access tokens;
* refresh tokens;
* API key full values;
* SMTP secrets;
* DKIM private keys;
* raw auth headers;
* full email bodies по умолчанию.

Соблюдать принцип ТЗ:

**structured logs, минимум чувствительных данных.**

---

# 58. ERROR HANDLING

Создать нормальную error taxonomy.

Frontend не должен показывать:

`Internal Server Error`

если можно показать:

`Mailbox provisioning failed`

`DNS verification failed`

`Mail service unavailable`

`Message temporarily deferred`

При этом техническая причина должна быть доступна ADMIN через logs/trace.

---

# 59. STALWART FAILURE UX

Особенно проверить сценарий:

**Stalwart DOWN.**

Система не должна:

* зависать бесконечно;
* показывать fake success;
* терять drafts;
* ломать Admin UI.

Compose должен корректно получить failure/deferred state.

Admin должен увидеть infrastructure incident.

---

# 60. WORKER FAILURE UX

Проверить:

**Worker DOWN.**

Accepted messages не должны бесследно исчезать.

Queue должна сохранять работу согласно текущей архитектуре.

После восстановления worker обработка должна продолжиться.

Добавить соответствующий integration test.

---

# 61. NATS FAILURE

Проверить degradation behavior.

Control Plane не должен падать целиком из-за одной инфраструктурной зависимости без понятного состояния.

Health должен показать проблему.

Логи должны содержать correlation ID.

---

# 62. CORRELATION / TRACE ID

Добавить/проверить correlation IDs для критических операций.

Особенно:

* API request;
* message acceptance;
* queue event;
* delivery;
* Stalwart interaction;
* webhook.

ADMIN Message Trace должен позволять проследить письмо через систему.

---

# 63. UTC

Согласно ТЗ внутреннее время системы должно храниться в UTC.

Frontend преобразует время для отображения.

Проверь это во всех новых модулях.

Не сохранять локальное браузерное время как authoritative timestamp.

---

# 64. PAGINATION

Не загружать тысячи сущностей сразу.

Проверить server-side pagination для:

* Inbox;
* Sent;
* Logs;
* Audit;
* Users;
* Domains;
* Mailboxes;
* Message Trace;
* Quarantine;
* Webhook deliveries.

---

# 65. SEARCH

Провести второй проход поиска.

Global Search должен находить минимум:

* mail;
* contacts;
* users;
* departments.

ADMIN search дополнительно:

* domains;
* mailbox;
* Message ID.

Command Palette использовать тот же search layer там, где это разумно.

---

# 66. EMPTY STATES

Новые инфраструктурные страницы не должны выглядеть пустыми.

Пример Domains empty state:

`No domains configured`

краткое объяснение

`Add domain`

Но не использовать огромную иллюстрацию на половину экрана.

---

# 67. LOADING STATES

Использовать skeletons, соответствующие реальной структуре страницы.

Не использовать один глобальный spinner на весь экран при каждой операции.

---

# 68. OPTIMISTIC UI

Использовать optimistic updates только там, где rollback понятен.

Например:

* star;
* read/unread;
* archive.

Не использовать optimistic success для:

* domain provisioning;
* mailbox creation;
* broadcast;
* security release;
* credential rotation.

---

# 69. ACCESSIBILITY V2

Повторно проверить:

* keyboard;
* focus;
* aria;
* modal focus trap;
* drawer;
* dropdown;
* table;
* Command Palette;
* Compose;
* Reading Pane.

---

# 70. MOBILE / TABLET

Webmail должен оставаться usable.

Desktop:

`Sidebar | List | Reading Pane`

Tablet:

`Sidebar collapsed | List | Pane`

Mobile:

`List → Message`

Compose:

full-screen.

Admin infrastructure pages могут быть desktop-first, но не должны полностью ломаться на небольшом viewport.

---

# 71. SECURITY TESTING

Добавить tests минимум для:

* USER cannot access ADMIN;
* disabled user cannot authenticate;
* revoked session rejected;
* revoked API key rejected;
* cross-user mailbox access denied;
* cross-organization access denied, если Organizations реализованы;
* attachment authorization;
* audit access ADMIN only;
* quarantine ADMIN only;
* Stalwart admin credential never exposed to frontend.

---

# 72. STALWART INTEGRATION TESTS

Добавить реальные integration tests.

Минимум:

### Test 1

Create domain → provision Stalwart.

### Test 2

Create mailbox → account exists.

### Test 3

Disable mailbox → authentication/use blocked.

### Test 4

Send internal mail A → B.

### Test 5

Send mail A → A.

### Test 6

Inbound message → mailbox Inbox.

### Test 7

Attachments survive flow.

### Test 8

Stalwart unavailable → correct failure.

### Test 9

Restart Stalwart → persistent mailbox/data state.

### Test 10

Audit contains provisioning actions without secrets.

---

# 73. MAIL FLOW E2E

Добавить полноценный E2E сценарий:

**ADMIN**

→ creates domain

→ verifies/provisions domain

→ creates users/mailboxes

→ USER A logs in

→ USER A sends mail to USER B

→ USER B receives mail

→ opens Reading Pane

→ replies

→ USER A receives reply

→ thread displays correctly.

Это один из главных acceptance tests Business Mail.

---

# 74. SELF-MAIL E2E

Отдельный mandatory E2E:

**USER A → USER A**

Проверить:

* Sent;
* Inbox;
* unread;
* thread;
* attachments;
* reply;
* no broken constraints.

---

# 75. SMTP E2E

Проверить submission через SMTP client:

* authentication;
* STARTTLS;
* accepted message;
* queue;
* recipient delivery;
* event visibility.

Не считать REST compose единственным mail flow.

---

# 76. RESTART TESTS

После интеграции проверить restart:

* API;
* worker;
* Stalwart;
* PostgreSQL;
* Redis;
* NATS.

Особенно:

* drafts;
* queued messages;
* mailboxes;
* attachments;
* events.

---

# 77. DATA MIGRATIONS

Все изменения схемы делать migrations.

Не редактировать production schema вручную.

Migration должна быть:

* deterministic;
* reviewable;
* testable.

Где возможно — предусмотреть rollback strategy.

---

# 78. SECRETS

Не добавлять secrets в `.env.example`.

Там только placeholder.

Production architecture должна быть совместима с Vault/secret manager, как предусмотрено ТЗ.

---

# 79. STALWART ADMIN UI

Не делай Stalwart Admin UI основной административной панелью продукта.

Наш Admin Portal остается главным интерфейсом.

Stalwart UI может существовать как internal/advanced maintenance tool, но пользователь не должен прыгать между двумя продуктами для обычных операций.

---

# 80. BRAND CONSISTENCY

Пользователь не должен ощущать, что внутри приложения «вставлен Stalwart».

Stalwart — инфраструктурный engine.

В пользовательском UI:

`Mail`

`Domains`

`Mailboxes`

`Delivery`

а не:

`Stalwart account #238`.

Название Stalwart допустимо в:

**Admin → Infrastructure**

и технической диагностике.

---

# 81. SECOND DESIGN PASS

После backend/infrastructure работ проведи еще один полный UI polish.

Не меняй дизайн-систему полностью.

Улучши текущую Silver/Graphite систему.

Особое внимание:

* alignment;
* density;
* typography;
* table headers;
* hover states;
* selected rows;
* pane separators;
* scrollbars;
* badges;
* toolbar hierarchy;
* modal sizes;
* drawers;
* form spacing;
* empty states;
* loading states.

---

# 82. УБРАТЬ AI-GENERATED UI PATTERNS

Повторно пройти приложение и удалить всё, что выглядит шаблонно:

* gradient headline;
* random colored icons;
* huge statistic cards;
* oversized radius;
* excessive shadows;
* decorative copy;
* fake quotes;
* unnecessary hero blocks;
* excessive pills;
* giant whitespace.

Интерфейс должен выглядеть как продукт, которым администратор реально управляет инфраструктурой каждый день.

---

# 83. MICROCOPY

Переписать плохие системные тексты.

Не:

`Oops! Something went wrong!`

А:

`Mail service is temporarily unavailable.`

`Your draft has been saved. Try sending again when the service recovers.`

ADMIN:

`Stalwart did not respond to the health check.`

`Last successful check: 14:32:08.`

Тексты короткие, технически точные и полезные.

---

# 84. НИКАКИХ ФИКТИВНЫХ METRICS

Очень важно.

Не показывать:

* fake delivery rate;
* random security score;
* fake uptime;
* fake latency;
* random message count;
* fake Stalwart status.

Если данных нет:

`No data`

или:

`Not enough data`.

---

# 85. ADMIN DETAILS DRAWERS

Для operational UI активно использовать side drawers.

Например:

* log detail;
* user detail;
* mailbox detail;
* security event;
* webhook delivery;
* message event.

Это позволяет не терять контекст списка.

---

# 86. URL STATE

Сохранять selection в URL там, где полезно.

Например:

`/mail/inbox?m=...`

`/admin/logs?event=...`

`/admin/domains?domain=...`

Это позволяет:

* refresh;
* deep link;
* back/forward.

---

# 87. PERMISSIONS

После появления новых modules расширить permission checks.

Не ограничиваться:

`if role === admin`.

Создай централизованную модель capabilities там, где это соответствует текущей архитектуре.

Но пока продукт официально использует USER/ADMIN — не создавай визуально десятки ролей без необходимости.

Архитектура может быть готова к будущему RBAC.

---

# 88. DATABASE INDEXES

После добавления operational queries проверить indexes.

Особенно:

* messages;
* mailbox relations;
* events;
* audit;
* logs;
* domains;
* users;
* quarantine.

Не добавлять индексы вслепую.

Оценить реальные query patterns.

---

# 89. PERFORMANCE TEST

Проверить интерфейс хотя бы на реалистичных объемах:

* 10k messages;
* 1k users;
* 100 domains;
* 100k audit events.

UI не должен пытаться отрисовать всё одновременно.

---

# 90. DOCUMENTATION

Обновить README/операционные документы.

Добавить:

### Local development

Как поднять:

* PostgreSQL;
* Redis;
* NATS;
* MinIO;
* Stalwart;
* Rspamd/ClamAV, если добавлены;
* API;
* worker;
* frontend.

### Stalwart

* configuration;
* volumes;
* ports;
* health;
* provisioning;
* troubleshooting.

### Mail flow

Нарисовать textual flow:

`Webmail → API → Mail Core → Queue → Delivery`

и inbound flow.

---

# 91. НЕ ПОВТОРЯТЬ ПРОБЛЕМУ С WORKER.EXE

Предыдущий этап обнаружил Windows App Control issue со свежесобранным `worker.exe`.

Проверь development workflow.

Если `go run ./cmd/worker` является надежным способом запуска в dev — задокументируй его.

Не строй production architecture вокруг этого workaround.

Production deployment должен запускать нормальный container/binary.

---

# 92. TEST MATRIX

После реализации прогнать:

### Build

* frontend build;
* Go build;
* worker build.

### Static

* TypeScript;
* lint;
* Go vet;
* formatter.

### Unit

* frontend relevant tests;
* backend domain tests.

### Integration

* PostgreSQL;
* Redis;
* NATS;
* MinIO;
* Stalwart;
* SMTP.

### E2E

* Login;
* Inbox;
* Compose;
* Self-mail;
* Reply;
* Attachments;
* Admin;
* Domain;
* Mailbox;
* Message Trace;
* Logs;
* Quarantine.

---

# 93. НЕ ПРОСТО ЗАПУСТИТЬ ТЕСТЫ — ИСПРАВИТЬ

Если старый тест сломался из-за правильного изменения архитектуры — обнови тест.

Если тест выявил regression — исправь regression.

Не отключай тест только для зеленого CI.

Не использовать `.skip` для проблемных critical flows.

---

# 94. SCREENSHOT QA

После завершения снова открыть живое приложение и сделать визуальный проход.

Проверить минимум:

* Login;
* Inbox empty;
* Inbox populated;
* Reading Pane;
* Compose;
* Contacts;
* Reminders;
* Admin Dashboard;
* Users;
* Domains;
* Domain Detail;
* Mailboxes;
* Message Trace;
* Security;
* Quarantine;
* Logs;
* Infrastructure.

Проверить минимум:

* 1440px;
* laptop viewport;
* tablet для mail;
* mobile для mail.

---

# 95. BROWSER CONSOLE

На финальном проходе не должно быть:

* React key warnings;
* hydration errors;
* uncontrolled input warnings;
* failed requests из-за неправильных routes;
* бесконечных retries;
* CORS ошибок;
* unnecessary 404;
* uncaught promises.

---

# 96. NETWORK QA

Проверить network requests.

Не должно быть:

* polling каждые 100 ms;
* одинаковых duplicate queries;
* N+1 запросов по каждому письму;
* загрузки всех audit events;
* постоянного health spam.

Использовать разумные cache/revalidation intervals.

---

# 97. DEFINITION OF DONE ЭТОГО ЭТАПА

Этап нельзя считать завершенным просто потому, что:

`Stalwart container is running`.

Этап считается успешным, когда минимум:

1. Stalwart встроен в local infrastructure.
2. Есть health integration.
3. Есть backend abstraction/provider.
4. Domains можно provisioning в mail-core.
5. Mailboxes можно provisioning в mail-core.
6. Internal mail A → B реально проходит.
7. Self-mail A → A исправлен.
8. Mail сохраняется после restart.
9. USER может читать mail через текущий Webmail.
10. SMTP submission работает.
11. Message lifecycle виден ADMIN.
12. Message Trace работает.
13. Stalwart failure корректно обрабатывается.
14. Secrets не попадают frontend/logs.
15. Audit фиксирует administrative provisioning.
16. Tests подтверждают integration.
17. UI остается единым и качественным.

---

# 98. ПРИОРИТЕТЫ

## P0 — ОБЯЗАТЕЛЬНО

* Stalwart integration;
* StalwartProvider abstraction;
* infrastructure;
* health;
* Domains;
* Mailboxes;
* provisioning;
* SMTP;
* JMAP/mailbox integration;
* self-mail fix;
* message lifecycle;
* Message Trace;
* secrets;
* audit;
* tests.

## P1

* DNS verification;
* DKIM;
* SMTP Credentials;
* API Keys;
* Webhooks;
* Rspamd;
* ClamAV;
* Quarantine;
* delivery analytics;
* Organizations/Projects;
* queue observability.

## P2

* Domain Health;
* Outbound Abuse improvements;
* advanced reputation;
* schedule send;
* aliases/shared mailboxes;
* advanced IMAP/JMAP client settings;
* additional analytics.

Не делай P2 вместо незавершенного P0.

---

# 99. АРХИТЕКТУРНЫЙ ПРИНЦИП

После этого этапа проект должен двигаться к следующей модели:

```text
                        ┌──────────────────────┐
                        │      Next.js UI      │
                        │ Webmail + Admin UI   │
                        └──────────┬───────────┘
                                   │
                        ┌──────────▼───────────┐
                        │       Go API         │
                        │    Control Plane     │
                        └───────┬──────┬───────┘
                                │      │
                     ┌──────────┘      └───────────┐
                     │                             │
              ┌──────▼──────┐              ┌──────▼──────┐
              │ PostgreSQL  │              │    Redis    │
              └─────────────┘              └─────────────┘

                     │
              ┌──────▼──────┐
              │    NATS     │
              │  JetStream  │
              └──────┬──────┘
                     │
              ┌──────▼──────┐
              │   Workers   │
              └──────┬──────┘
                     │
              ┌──────▼─────────────────────┐
              │       Mail Core Layer      │
              │                            │
              │         Stalwart           │
              │ SMTP / JMAP / IMAP / Mail │
              └──────┬─────────────────────┘
                     │
            ┌────────▼─────────┐
            │ Security Pipeline│
            │ Rspamd / ClamAV  │
            └────────┬─────────┘
                     │
              ┌──────▼──────┐
              │ External MX │
              └─────────────┘

       MinIO/S3 ← attachments / artifacts / quarantine
```

Не воспринимай эту схему как требование слепо связать все компоненты именно последовательной цепочкой. Сохраняй правильные boundaries и адаптируй схему к фактической архитектуре repository.

---

# 100. ГЛАВНЫЙ РЕЗУЛЬТАТ

После второго этапа приложение должно перестать быть просто:

**«красивым корпоративным почтовым интерфейсом».**

Оно должно начать становиться:

**полноценной корпоративной Mail Platform с собственным Control Plane и реальным Mail Core.**

Обычный USER должен видеть простой продукт:

> Inbox → Read → Reply → Compose → Contacts → Reminders.

ADMIN должен видеть значительно более глубокий уровень:

> Users → Departments → Organizations → Domains → Mailboxes → Delivery → Message Trace → Security → Logs → Infrastructure.

А под этим должна реально работать инфраструктура:

> Go + PostgreSQL + Redis + NATS + MinIO + Stalwart + Workers + Security Pipeline.

---

# 101. ФИНАЛЬНЫЙ ОТЧЕТ АГЕНТА

После завершения НЕ отвечай общими словами вроде:

`Everything has been improved successfully.`

Предоставь структурированный технический отчет:

### 1. Architecture

Что изменено и почему.

### 2. Stalwart

Что именно интегрировано:

* SMTP;
* JMAP;
* IMAP;
* provisioning;
* domains;
* mailboxes;
* health.

### 3. Database

Какие migrations добавлены.

### 4. Backend

Какие handlers/services/providers/events добавлены или изменены.

### 5. Frontend

Какие страницы/компоненты добавлены или переработаны.

### 6. Infrastructure

Docker/ports/volumes/configuration.

### 7. Security

Secrets, permissions, quarantine, scanning.

### 8. Bugs Fixed

Особенно self-mail и найденные regressions.

### 9. Tests

Указать точные результаты:

* unit;
* integration;
* UI;
* E2E;
* Stalwart;
* SMTP.

### 10. Remaining Gaps

Что действительно осталось нереализованным.

### 11. Manual Setup

Что должен сделать разработчик после получения изменений.

### 12. Production Risks

Что пока нельзя считать production-ready.

Не скрывай проблемы ради красивого отчета.

---

# ФИНАЛЬНАЯ КОМАНДА

**Начни с повторного анализа ТЗ и текущего repository. Затем составь gap analysis, после чего реализуй P0 последовательно. Не переписывай уже работающий проект с нуля. Интеграция Stalwart должна быть реальной и проверенной end-to-end. После каждого крупного архитектурного изменения запускай соответствующие тесты. После завершения проведи UI/UX polish и полный QA живого приложения.**

Главный критерий:

> **Не просто подключить Stalwart. Сделать Stalwart частью архитектуры продукта, сохранив наш Go backend как Control Plane и наш интерфейс как единую точку управления платформой.**
