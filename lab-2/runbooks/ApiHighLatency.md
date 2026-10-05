# ApiHighLatency

**Severity:** critical

**Условие:** p95 времени ответа одного маршрута `api` выше 2 секунд три
минуты подряд, при трафике на этот маршрут не меньше 0.05 req/s. Алерт
приходит **по маршруту** — метка `route`.

## Что это значит для пользователя

Каждый двадцатый запрос к этому маршруту ждёт дольше двух секунд. Часть
клиентов уже упирается в таймауты, а повторы от них добавляют нагрузку.

## 1. Подтвердить

Дашборд **api - RED**, панель *Duration p95 by route*, или:

```bash
kubectl -n monitoring port-forward svc/kps-prometheus 19090:9090 &
q() { curl -s http://127.0.0.1:19090/api/v1/query --data-urlencode "query=$1"; }

# p95 по маршрутам
q 'histogram_quantile(0.95, sum by (route, le) (rate(http_request_duration_seconds_bucket{job="api"}[5m])))'

# вырос ли трафик на этот маршрут (перегрузка) или он прежний (что-то замедлилось)
q 'sum by (route) (rate(http_requests_total{job="api"}[5m]))'
```

## 2. Где уходит время

Метрики говорят «медленно», трейс — «где именно». В Jaeger
(`kubectl -n monitoring port-forward svc/jaeger 16686:16686`,
http://localhost:16686): Service `api`, Operation = маршрут из алерта,
**Min Duration** `2s` → Find Traces. В водопаде видно, какой спан занимает
время: собственный код, вложенная операция (как `slow-op`) или вызов
зависимости.

## 3. Не упираемся ли в ресурсы

```bash
# сколько CPU под реально тратит (в ядрах) — сравнить с requests
q 'sum(rate(container_cpu_usage_seconds_total{namespace="app", container="api"}[5m]))'

# CPU throttling — только если у контейнера задан cpu limit (в нашем чарте
# его нет, и тогда этих счётчиков просто не существует); высокий — упираемся в limit
q 'sum(rate(container_cpu_cfs_throttled_periods_total{namespace="app", container="api"}[5m])) / sum(rate(container_cpu_cfs_periods_total{namespace="app", container="api"}[5m]))'

# память пода относительно лимита
q 'sum(container_memory_working_set_bytes{namespace="app", container="api"}) / sum(kube_pod_container_resource_limits{namespace="app", container="api", resource="memory"})'

kubectl -n app get pods
kubectl -n app describe pod -l app.kubernetes.io/name=api | grep -A3 -E 'Limits|Requests'
```

## 4. Как чинить

| Причина | Действие |
|---|---|
| Замедление после деплоя | откатить: `helm -n app rollback api` |
| Трафик вырос, под упирается в CPU | добавить реплик: `kubectl -n app scale deploy/api --replicas=3` (постоянно — через values), поднять `requests`/`limits` |
| Медленная зависимость (видно по спану) | эскалировать владельцам зависимости; у себя — таймаут и деградация вместо ожидания |
| Маршрут медленный по природе (тяжёлый отчёт) | это не инцидент, а неверный порог: задать для маршрута свой порог или вынести в асинхронную задачу |

## 5. После

- p95 вернулся ниже порога, алерт ушёл в resolved.
- Если порог для маршрута неверный — поправить правило, а не глушить алерт
  навсегда.
