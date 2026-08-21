Отчёт V3
1. Executive Summary
Закрыты все четыре критических гейта (§135–138) — то, что вы поставили условием этапа. Два mail store стали одним. Плюс закрыты оба фронтенд-долга и миграция legacy.

2–4. Unified Mail Architecture / Source of Truth / Bridge
Authoritative для содержимого ящиков — Stalwart; PostgreSQL остаётся control plane (пользователи, домены, политики, аудит, trace). Мост: Next.js → Go API → internal/mailservice → JMAP, аутентификация master-user'ом (<account>%master), админские креды в браузер не уходят. Dual-write отсутствует: на одну доставку ровно одна авторитетная запись. Зафиксировано в ADR-003.

5. Gate №1 — одно письмо, три интерфейса

Webmail:  id=biaaaaak   rfc=2eaa5086-…@company.test
JMAP:     id=biaaaaak
IMAP:     Message-ID: <2eaa5086-1437-4994-a4bf-8a265a313378@company.test>
Флаги — протокольные: отметка в Webmail → IMAP отдаёт FLAGS (\Flagged \Seen).

6–7. Gate №2 — inbound
Письмо, поданное снаружи на порт 25 без аутентификации, принято, классифицировано (нет SPF/DKIM/DMARC → Junk) и видно одновременно в Webmail spam и IMAP Junk Mail. Неизвестный получатель отклоняется на RCPT: 550 5.1.2 — не «принять и выбросить» (§66).

8. Gate №3 — outbound
Контролируемая цель cmd/smtpsink (профиль test, алиас sink.test):

Получатель	Ответ	Результат
ok-…	250	доставлено, есть в логе приёмника
defer-…	451	в очереди, retry_num растёт 1→2→3, ответ сохранён дословно
bounce-…	550	DSN отправителю с цитатой отказа, без ретраев
9–10. DNS / Security
DKIM не реализован (DKIM signer not found в логах — исходящее не подписано). Rspamd/ClamAV не развёрнуты; классификация идёт от встроенного фильтра Stalwart. Их failure-policy сознательно не выбрана «на глаз» — по §108 требует ADR.

11–13. Queue / Trace / Provisioning
Очередь и ретраи принадлежат mail core; метаданные попыток (номер, время, код, текст, next_retry) читаются через management API. Provisioning переведён на provisioning_jobs + воркер с backoff — синхронных вызовов в HTTP-хендлерах больше нет.

14. Credential Hardening — найдена и закрыта реальная утечка
Тесты редакции (§106) поймали настоящую уязвимость: если mail core возвращает в теле ошибки эхо заголовков, base64 с мастер-паролем попадал в строки ошибок и логи. Исправлено редакцией ответов на границе клиента; 4 теста подтверждают, что секрет не появляется ни в ошибках, ни в логах, ни в URL/теле, ни в JSON для браузера.

15. Migrations
00018_unified_mail.sql — статус relayed, sent_imported_at, provisioning_jobs, migrated_at.

16–17. Frontend / Infrastructure
Оба долга, о которых я предупреждал, оказались уже закрыты в коде — я это проверил вживую, а не по коду: вложение с юникодным именем отчёт-есеп.txt прошло сквозняком (55 байт, точный UTF-8 с казахским), черновик при обновлении меняет id и оставляет ровно одну копию. Добавлен контейнер smtpsink (профиль test), healthcheck Stalwart починен под образ без curl.

18. Tests
Слой	Результат
go build / go vet / gofmt	чисто
go test ./...	ok (+4 новых теста редакции)
tsc / eslint / next build	чисто
E2E	60/60 passed (+14 новых)
19. Failure tests
Stalwart DOWN: webmail → 503 MAIL_SERVICE_UNAVAILABLE (не 500), отправка принимается durable, control plane жив, health честно unavailable. После рестарта — письмо доставлено и provisioning завершён сам (3 попытки → done), без правки БД.

20. Internet Verification — честно
Публичный интернет не проверялся вообще. Нет публичного IP, входящего порта 25 и PTR. Всё inbound/outbound доказано только в контролируемом окружении (локальный SMTP-приёмник и локальная подача на порт 25). Называть Internet Mail работающим я не имею права.

21. Remaining Gaps
DKIM; DNS-верификация; Rspamd/ClamAV; Queue Center и Deliverability UI (данные есть, экранов нет); Organizations/Projects иерархия; shared mailboxes.

22. Production Risks
Self-signed TLS; app-пароли уходят в Stalwart plaintext'ом внутри secret (TECH_DEBT #11); импорт в store — at-least-once, краш между импортом и коммитом статуса может продублировать копию (лечится -dedupe); один воркер; Stalwart запинен на v0.13.4 (v0.16+ ломает воспроизводимую конфигурацию); AGPL при дистрибуции — нужна отдельная юридическая оценка.

23. Manual Setup
docker compose up -d, make migrate-up, make migrate-mail, перезапуск api/worker. Для тестов доставки — make sink-up. Новые переменные (STALWART_MASTER_*, STALWART_SUBMIT_ADDR) уже в .env и .env.example.

Замечу отдельно: в процессе я сам создал 25 дубликатов, сбросив метки миграции при незакрытом баге сравнения Message-ID (PostgreSQL хранит в угловых скобках, JMAP отдаёт без). Баг исправлен, дубликаты вычищены -dedupe, идемпотентность доказана повторным прогоном.