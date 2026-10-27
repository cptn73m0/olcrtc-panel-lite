# Форк olcrtc-panel-lite

Форк [juushimatsu/olcrtc-panel-lite](https://github.com/juushimatsu/olcrtc-panel-lite).
Апстрим встал после того, как WB Stream закрыл авторизацию для не-сотрудников:
вся автоматизация панели строилась вокруг этого провайдера.

## Что делаем

Панель приводится в вид, пригодный для долгой эксплуатации на своем сервере:
чиним хранение статистики, размер журналов, TLS, наблюдаемость и дефолты.

Полный список найденных дефектов с замерами - [docs/fork/ops-backlog.md](docs/fork/ops-backlog.md).

## Что выпиливаем

- WB Stream: `worker.mjs`, `evade.cjs`, встроенный Node, noVNC и Playwright,
  настройки `wb_token` и `wb_session_*`, шаги автонастройки с `playwright_check`.
- Автообновление с релизного канала апстрима: канал больше не обновляется.

## Сборка

```sh
make build     # build/olcrtc-panel
make check     # vet + test-race + node --check + bash -n
```

## Правила

1. `ub-mode`: сначала работает, потом правильно, потом практика.
2. Код и коммиты на английском, документация на русском.
3. `make check` перед каждым коммитом.
4. Изменения - короткими ветками, журнал в [docs/fork/log.md](docs/fork/log.md).
