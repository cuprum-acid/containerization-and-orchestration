# ApiHighErrorRate

**Severity:** critical

**Условие:** больше 5% запросов к `api` заканчиваются ответом 5xx две минуты
подряд, при трафике не меньше 0.1 req/s.

## Что это значит для пользователя

Каждый двадцатый запрос и больше падает с ошибкой. Пользователь видит
ошибку прямо сейчас; клиенты с повторами создают ещё больше нагрузки.

## 1. Подтвердить и оценить масштаб

Какая доля ошибок сейчас и на каких маршрутах — дашборд **api - RED**,
панели *Errors* и *Error ratio*, или напрямую:

```bash
kubectl -n monitoring port-forward svc/kps-prometheus 19090:9090 &
q() { curl -s http://127.0.0.1:19090/api/v1/query --data-urlencode "query=$1"; }

# доля ошибок по маршрутам
q 'sum by (route) (rate(http_request_errors_total{job="api"}[2m])) / sum by (route) (rate(http_requests_total{job="api"}[2m]))'

# какие именно коды
q 'sum by (route, code) (rate(http_requests_total{job="api", code=~"5.."}[2m]))'
```

- Ошибки на одном маршруте — скорее всего, проблема в его коде или в
  зависимости, которую вызывает только он.
- Ошибки на всех маршрутах — проблема с подом, конфигом или общей
  зависимостью.

## 2. Что изменилось

```bash
# был ли недавний деплой
helm -n app history api
kubectl -n app rollout history deploy/api

# состояние подов: рестарты, OOMKilled, CrashLoopBackOff
kubectl -n app get pods -o wide
kubectl -n app describe pod -l app.kubernetes.io/name=api | grep -A5 -E 'Last State|Events'
```

## 3. Найти саму ошибку

Логи ошибок в Grafana → Explore → Loki:

```logql
{namespace="app", app="api", level="ERROR"}
```

Самые частые тексты ошибок за 5 минут:

```logql
sum by (error) (count_over_time({namespace="app", app="api", level="ERROR"} | json | error != "" [5m]))
```

Раскрыть строку → **Trace in Grafana** — трейс этого запроса: красный спан
показывает, на каком шаге произошла ошибка, в атрибутах спана — текст
исключения.

## 4. Как чинить

| Причина | Действие |
|---|---|
| Ошибки начались сразу после деплоя | откатить: `helm -n app rollback api` (или `kubectl -n app rollout undo deploy/api`), потом разбираться |
| Под в CrashLoop / OOMKilled | см. логи прошлого запуска `kubectl -n app logs <pod> --previous`; при OOM поднять `resources.limits.memory` в values |
| Ошибки от зависимости (БД, другой сервис) | проверить её состояние; если она лежит — эскалировать её владельцам, у себя включить деградацию/фичефлаг |
| Ошибки на одном маршруте из-за плохих входных данных | это не 5xx-ситуация: такие ответы должны быть 4xx — завести задачу на исправление |

## 5. После

- Убедиться, что доля ошибок вернулась ниже порога и алерт ушёл в resolved
  (Karma, Alertmanager).
- Если причина — деплой: добавить проверку в CI или канареечный выкат.
