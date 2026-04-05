
# k8s-cli

CLI утилита для управления Kubernetes на Go.

## Установка

```bash
# Сборка
go mod tidy
go build -o k8s-cli

# Перемещение в PATH (опционально)
sudo mv k8s-cli /usr/local/bin/
```

## Требования

- Go 1.21+
- Настроенный доступ к Kubernetes cluster (`~/.kube/config`)
- kubectl (для проверки конфигурации)

---

## Команды

### 1. get pods

Получение списка подов с фильтрацией.

```bash
./k8s-cli get pods [flags]
```

**Aliases:** `pod`, `po`

**Флаги:**

| Флаг | Короткий | Описание | Пример |
|------|----------|----------|--------|
| `--namespace` | `-n` | Namespace для поиска | `-n kube-system` |
| `--all-namespaces` | `-A` | Поиск по всем namespace | `-A` |
| `--status` | `-s` | Фильтр по статусу | `-s Running` |
| `--help` | `-h` | Показать справку | `-h` |

**Примеры:**

```bash
# Все поды в default namespace
./k8s-cli get pods

# Все поды во всех namespace
./k8s-cli get pods -A

# Только Running поды
./k8s-cli get pods -s Running

# Все поды кроме Running
./k8s-cli get pods -s not:Running

# В конкретном namespace
./k8s-cli get pods -n kube-system

# Комбинация фильтров
./k8s-cli get pods -n default -s Running

# Краткая форма
./k8s-cli get po -A
./k8s-cli get pod -s Pending
```

**Статусы подов:**

| Статус | Описание |
|--------|----------|
| `Running` | Под запущен |
| `Pending` | Под создан, но не запущен |
| `Failed` | Под завершил работу с ошибкой |
| `Succeeded` | Под завершил работу успешно |
| `Unknown` | Статус неизвестен |

---

### 2. get nodes

Получение списка узлов кластера.

```bash
./k8s-cli get nodes [flags]
```

**Aliases:** `node`, `no`

**Флаги:**

| Флаг | Короткий | Описание | Пример |
|------|----------|----------|--------|
| `--status` | `-s` | Фильтр по статусу | `-s Ready` |
| `--zone` | `-z` | Фильтр по зоне | `-z us-east-1` |
| `--selector` | `-l` | Селектор лейблов | `-l node-role.kubernetes.io/worker=` |
| `--help` | `-h` | Показать справку | `-h` |

**Примеры:**

```bash
# Все узлы
./k8s-cli get nodes

# Только Ready узлы
./k8s-cli get nodes -s Ready

# Все узлы кроме Ready
./k8s-cli get nodes -s not:Ready

# Узлы в конкретной зоне
./k8s-cli get nodes -z us-east-1

# Worker узлы
./k8s-cli get nodes -l node-role.kubernetes.io/worker=

# Комбинация фильтров
./k8s-cli get nodes -z us-east-1 -s Ready -l node-role.kubernetes.io/worker=

# Краткая форма
./k8s-cli get no
```

**Статусы узлов:**

| Статус | Описание |
|--------|----------|
| `Ready` | Узел готов к работе |
| `NotReady` | Узел не готов |
| `Unknown` | Статус неизвестен |

---

### 3. delete pods

Удаление подов по статусу.

```bash
./k8s-cli delete pods [flags]
```

**Флаги:**

| Флаг | Короткий | Описание | Пример |
|------|----------|----------|--------|
| `--namespace` | `-n` | Namespace для поиска | `-n default` |
| `--all-namespaces` | `-A` | Поиск по всем namespace | `-A` |
| `--status` | `-s` | Фильтр по статусу **(обязательно)** | `-s Failed` |
| `--force` | `-f` | Пропустить подтверждение | `-f` |
| `--help` | `-h` | Показать справку | `-h` |

**Примеры:**

```bash
# Удалить все Failed поды
./k8s-cli delete pods -s Failed

# Удалить все Failed поды во всех namespace
./k8s-cli delete pods -s Failed -A

# Удалить все поды кроме Running (осторожно!)
./k8s-cli delete pods -s not:Running -A

# Без подтверждения (для CI/CD)
./k8s-cli delete pods -s Failed -f

# В конкретном namespace
./k8s-cli delete pods -n kube-system -s Pending
```

> ⚠️ **Предупреждение:** Команда требует подтверждения перед удалением. Используйте `-f` с осторожностью!

---

### 4. cordon

Пометить узел(ы) как недоступные для планирования.

```bash
./k8s-cli cordon [node-name] [flags]
```

**Флаги:**

| Флаг | Короткий | Описание | Пример |
|------|----------|----------|--------|
| `--zone` | `-z` | Пометить все узлы в зоне | `-z us-east-1` |
| `--force` | `-f` | Пропустить подтверждение | `-f` |
| `--help` | `-h` | Показать справку | `-h` |

**Примеры:**

```bash
# Пометить конкретный узел
./k8s-cli cordon worker-node-1

# Пометить все узлы в зоне
./k8s-cli cordon -z us-east-1

# Без подтверждения
./k8s-cli cordon worker-node-1 -f
./k8s-cli cordon -z us-east-1 -f
```

> **Результат:** Новые поды не будут размещаться на помеченных узлах.

---

### 5. uncordon

Пометить узел(ы) как доступные для планирования.

```bash
./k8s-cli uncordon [node-name] [flags]
```

**Флаги:**

| Флаг | Короткий | Описание | Пример |
|------|----------|----------|--------|
| `--zone` | `-z` | Пометить все узлы в зоне | `-z us-east-1` |
| `--force` | `-f` | Пропустить подтверждение | `-f` |
| `--help` | `-h` | Показать справку | `-h` |

**Примеры:**

```bash
# Вернуть конкретный узел
./k8s-cli uncordon worker-node-1

# Вернуть все узлы в зоне
./k8s-cli uncordon -z us-east-1

# Без подтверждения
./k8s-cli uncordon worker-node-1 -f
```

---

### 6. drain

Освободить узел(ы) для обслуживания.

```bash
./k8s-cli drain [node-name] [flags]
```

**Флаги:**

| Флаг | Короткий | Описание | Пример |
|------|----------|----------|--------|
| `--zone` | `-z` | Освободить все узлы в зоне | `-z us-east-1` |
| `--force` | `-f` | Пропустить подтверждение | `-f` |
| `--ignore-daemonsets` | | Игнорировать DaemonSet поды | `--ignore-daemonsets` |
| `--delete-emptydir-data` | | Удалять поды с emptyDir | `--delete-emptydir-data` |
| `--timeout` | | Таймаут ожидания (сек) | `--timeout 600` |
| `--help` | `-h` | Показать справку | `-h` |

**Примеры:**

```bash
# Освободить конкретный узел
./k8s-cli drain worker-node-1

# С игнорированием DaemonSet
./k8s-cli drain worker-node-1 --ignore-daemonsets

# Полное освобождение без подтверждений
./k8s-cli drain worker-node-1 -f --ignore-daemonsets --delete-emptydir-data

# Освободить все узлы в зоне
./k8s-cli drain -z us-east-1 --ignore-daemonsets -f

# С увеличенным таймаутом (10 минут)
./k8s-cli drain worker-node-1 --timeout 600
```

**Процесс drain:**

1. Помечает узел как `unschedulable`
2. Вытесняет поды (evict)
3. Ждёт завершения удаления подов

---

## Сценарии использования

### 1. Обслуживание узлов в зоне

```bash
# 1. Проверка узлов в зоне
./k8s-cli get nodes -z us-east-1

# 2. Освобождение всех узлов в зоне
./k8s-cli drain -z us-east-1 --ignore-daemonsets -f

# 3. После обслуживания вернуть узлы
./k8s-cli uncordon -z us-east-1 -f
```

### 2. Очистка Failed подов

```bash
# 1. Просмотр Failed подов
./k8s-cli get pods -A -s Failed

# 2. Удаление Failed подов
./k8s-cli delete pods -s Failed -A -f
```

### 3. Поиск проблемных узлов

```bash
# Все узлы кроме Ready
./k8s-cli get nodes -s not:Ready

# Узлы в конкретной зоне со статусом NotReady
./k8s-cli get nodes -z us-east-1 -s NotReady
```

### 4. Плановое обслуживание

```bash
# 1. Пометить узел как недоступный
./k8s-cli cordon worker-node-1

# 2. Освободить узел от подов
./k8s-cli drain worker-node-1 --ignore-daemonsets -f

# ... выполнение работ ...

# 3. Вернуть узел в строй
./k8s-cli uncordon worker-node-1
```

---

## Структура проекта

```
k8s-cli/
├── cmd/
│   ├── root.go           # Корневая команда
│   ├── get.go            # Команда get
│   ├── get_pods.go       # get pods
│   ├── get_nodes.go      # get nodes
│   ├── delete.go         # Команда delete
│   ├── delete_pods.go    # delete pods
│   ├── cordon.go         # cordon
│   ├── uncordon.go       # uncordon
│   └── drain.go          # drain
├── pkg/
│   └── k8s/
│       └── client.go     # Kubernetes клиент
├── main.go
├── go.mod
├── go.sum
└── README.md
```

---

## Инверсия фильтров

Для инверсии фильтров используйте префикс `not:`:

```bash
# Все поды кроме Running
./k8s-cli get pods -s not:Running

# Все узлы кроме Ready
./k8s-cli get nodes -s not:Ready

# Все поды кроме Failed в namespace
./k8s-cli delete pods -s not:Failed -n default
```

> **Примечание:** В zsh используйте кавычки: `'-s not:Running'`

---

## Справка

Для получения справки по любой команде используйте флаг `-h` или `--help`:

```bash
./k8s-cli --help
./k8s-cli get --help
./k8s-cli get pods --help
./k8s-cli drain --help
```

---

## Лицензия

MIT

---

## Поддержка

Для вопросов и предложений создайте issue в репозитории проекта.
