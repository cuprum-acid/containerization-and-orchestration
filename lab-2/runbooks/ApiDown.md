# ApiDown

**Severity:** critical

**Условие:** у Prometheus нет ни одной доступной цели `api` минуту подряд
(`absent(up{job="api"} == 1)`).

## Что это значит

Одно из двух, и первое дело — понять, какое:

1. **Сервис лежит** — пользователи получают ошибки соединения. Самый
   срочный случай.
2. **Сервис работает, но мониторинг его не видит** — пользователи не
   страдают, но мы ослепли: остальные алерты по `api` не сработают.

## 1. Есть ли поды и готовы ли они

```bash
kubectl -n app get deploy,pods -l app.kubernetes.io/name=api
kubectl -n app get endpointslices -l kubernetes.io/service-name=api
kubectl -n app describe deploy api | grep -E 'Replicas|Conditions' -A3
kubectl -n app get events --sort-by=.lastTimestamp | tail -20
```

| Что видим | Что это |
|---|---|
| `replicas: 0` | кто-то отмасштабировал в ноль — `helm -n app history api`, `kubectl -n app rollout history deploy/api` |
| поды `Pending` | не хватает ресурсов на нодах или не найден образ: `describe pod`, события |
| поды `CrashLoopBackOff` | приложение падает при старте: `kubectl -n app logs <pod> --previous` |
| `ImagePullBackOff` / `ErrImageNeverPull` | нет образа: для minikube — `minikube image build -t api:lab2 .` |
| поды `Running`, но не `Ready` | не проходит readiness `/healthz`: `kubectl -n app describe pod` → Events |
| поды `Running` и `Ready` | сервис жив — проблема в сборе метрик, см. шаг 2 |

## 2. Если поды живы — почему Prometheus их не видит

```bash
# отвечает ли сервис сам
kubectl -n app port-forward svc/api 18080:8080 &
curl -s localhost:18080/health
curl -s localhost:18080/metrics | head

# есть ли ServiceMonitor и попал ли job в конфиг Prometheus
kubectl -n app get servicemonitor api
kubectl -n monitoring get secret prometheus-kps-prometheus \
  -o jsonpath='{.data.prometheus\.yaml\.gz}' | base64 -d | gunzip | grep 'serviceMonitor/app/api'

# что Prometheus думает о цели
kubectl -n monitoring port-forward svc/kps-prometheus 19090:9090 &
curl -s 'http://127.0.0.1:19090/api/v1/targets?state=active' | grep -A5 'app/api'
```

Частые причины: у Service поменялись метки или имя порта, и селектор
ServiceMonitor больше не совпадает; NetworkPolicy закрыла порт от
Prometheus; сломался сам Prometheus.

## 3. Как чинить

| Причина | Действие |
|---|---|
| Отмасштабировали в ноль | `kubectl -n app scale deploy/api --replicas=1`, а постоянно — `helm upgrade` с правильными values |
| Падает после деплоя | `helm -n app rollback api` |
| Нет образа | пересобрать и загрузить образ, `kubectl -n app rollout restart deploy/api` |
| Селектор ServiceMonitor не совпадает с Service | поправить метки/порт в чарте, `helm upgrade` |

## 4. После

- Цель снова `up`, алерт ушёл в resolved.
- Если причина — ручное действие в кластере (scale, delete), вернуть
  состояние через Helm, чтобы следующий `helm upgrade` его не перетёр.
