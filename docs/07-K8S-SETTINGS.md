# Kubernetes 배포 설정

## 로컬 개발 환경 (Kind/Minikube)

### 사전 요구사항

- Docker Desktop
- kubectl
- Kind or Minikube
- Helm (Optional)

### 시스템 리소스 요구사항

**최소**:
- CPU: 4 cores
- RAM: 8GB
- Disk: 30GB

**권장**:
- CPU: 8 cores
- RAM: 16GB
- Disk: 50GB

---

## 1. Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: link-archive
  labels:
    name: link-archive
    environment: local
```

---

## 2. ConfigMap (환경 설정)

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: link-archive
data:
  # Database
  POSTGRES_HOST: "postgresql.link-archive.svc.cluster.local"
  POSTGRES_PORT: "5432"
  POSTGRES_DB: "linkarchive"
  
  # Redis
  REDIS_HOST: "redis.link-archive.svc.cluster.local"
  REDIS_PORT: "6379"
  REDIS_DB: "0"
  
  # RabbitMQ
  RABBITMQ_HOST: "rabbitmq.link-archive.svc.cluster.local"
  RABBITMQ_PORT: "5672"
  RABBITMQ_VHOST: "/"
  
  # MinIO
  MINIO_ENDPOINT: "minio.link-archive.svc.cluster.local:9000"
  MINIO_USE_SSL: "false"
  MINIO_BUCKET_THUMBNAILS: "thumbnails"
  MINIO_BUCKET_CONTENT: "content-cache"
  
  # OpenAI
  OPENAI_MODEL: "gpt-4o-mini"
  OPENAI_MAX_TOKENS: "500"
  OPENAI_TEMPERATURE: "0.7"
  
  # App
  LOG_LEVEL: "info"
  ENVIRONMENT: "local"
  TZ: "Asia/Seoul"
```

---

## 3. Secret (민감 정보)

```yaml
# secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: app-secrets
  namespace: link-archive
type: Opaque
stringData:
  # PostgreSQL
  POSTGRES_USER: "linkuser"
  POSTGRES_PASSWORD: "securepassword123"
  
  # Redis
  REDIS_PASSWORD: "redispass123"
  
  # RabbitMQ
  RABBITMQ_USER: "admin"
  RABBITMQ_PASSWORD: "rabbitpass123"
  
  # MinIO
  MINIO_ACCESS_KEY: "minioadmin"
  MINIO_SECRET_KEY: "minioadmin123"
  
  # OpenAI
  OPENAI_API_KEY: "sk-proj-..."
  
  # JWT
  JWT_SECRET: "your-super-secret-jwt-key-change-this-in-production"
  JWT_REFRESH_SECRET: "your-super-secret-refresh-key-change-this-in-production"
```

**주의**: 프로덕션에서는 Sealed Secrets, External Secrets Operator 등 사용

---

## 4. PostgreSQL (StatefulSet)

```yaml
# postgresql.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgresql
  namespace: link-archive
spec:
  serviceName: postgresql
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
      - name: postgresql
        image: postgres:16-alpine
        ports:
        - containerPort: 5432
          name: postgres
        env:
        - name: POSTGRES_DB
          valueFrom:
            configMapKeyRef:
              name: app-config
              key: POSTGRES_DB
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: POSTGRES_USER
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: POSTGRES_PASSWORD
        - name: PGDATA
          value: /var/lib/postgresql/data/pgdata
        volumeMounts:
        - name: postgres-data
          mountPath: /var/lib/postgresql/data
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "2Gi"
            cpu: "1000m"
        livenessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - $(POSTGRES_USER)
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - $(POSTGRES_USER)
          initialDelaySeconds: 5
          periodSeconds: 5
  volumeClaimTemplates:
  - metadata:
      name: postgres-data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: postgresql
  namespace: link-archive
spec:
  selector:
    app: postgresql
  ports:
  - port: 5432
    targetPort: 5432
  clusterIP: None  # Headless service for StatefulSet
```

---

## 5. Redis (Deployment)

```yaml
# redis.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: link-archive
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: redis:7-alpine
        ports:
        - containerPort: 6379
          name: redis
        command:
        - redis-server
        - --requirepass
        - $(REDIS_PASSWORD)
        - --appendonly
        - "yes"
        env:
        - name: REDIS_PASSWORD
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: REDIS_PASSWORD
        volumeMounts:
        - name: redis-data
          mountPath: /data
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - redis-cli
            - ping
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: redis-data
        emptyDir: {}  # 로컬 개발용, 프로덕션에서는 PVC 사용
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: link-archive
spec:
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
  type: ClusterIP
```

---

## 6. RabbitMQ (Deployment)

```yaml
# rabbitmq.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: rabbitmq
  namespace: link-archive
spec:
  replicas: 1
  selector:
    matchLabels:
      app: rabbitmq
  template:
    metadata:
      labels:
        app: rabbitmq
    spec:
      containers:
      - name: rabbitmq
        image: rabbitmq:3-management-alpine
        ports:
        - containerPort: 5672
          name: amqp
        - containerPort: 15672
          name: management
        env:
        - name: RABBITMQ_DEFAULT_USER
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: RABBITMQ_USER
        - name: RABBITMQ_DEFAULT_PASS
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: RABBITMQ_PASSWORD
        volumeMounts:
        - name: rabbitmq-data
          mountPath: /var/lib/rabbitmq
        resources:
          requests:
            memory: "256Mi"
            cpu: "200m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          exec:
            command:
            - rabbitmq-diagnostics
            - ping
          initialDelaySeconds: 60
          periodSeconds: 30
        readinessProbe:
          exec:
            command:
            - rabbitmq-diagnostics
            - ping
          initialDelaySeconds: 20
          periodSeconds: 10
      volumes:
      - name: rabbitmq-data
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: rabbitmq
  namespace: link-archive
spec:
  selector:
    app: rabbitmq
  ports:
  - name: amqp
    port: 5672
    targetPort: 5672
  - name: management
    port: 15672
    targetPort: 15672
  type: ClusterIP
---
# RabbitMQ Management UI 접근용 (로컬 개발)
apiVersion: v1
kind: Service
metadata:
  name: rabbitmq-nodeport
  namespace: link-archive
spec:
  type: NodePort
  selector:
    app: rabbitmq
  ports:
  - name: management
    port: 15672
    targetPort: 15672
    nodePort: 30672
```

---

## 7. MinIO (StatefulSet)

```yaml
# minio.yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: minio
  namespace: link-archive
spec:
  serviceName: minio
  replicas: 1
  selector:
    matchLabels:
      app: minio
  template:
    metadata:
      labels:
        app: minio
    spec:
      containers:
      - name: minio
        image: minio/minio:latest
        args:
        - server
        - /data
        - --console-address
        - ":9001"
        ports:
        - containerPort: 9000
          name: api
        - containerPort: 9001
          name: console
        env:
        - name: MINIO_ROOT_USER
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: MINIO_ACCESS_KEY
        - name: MINIO_ROOT_PASSWORD
          valueFrom:
            secretKeyRef:
              name: app-secrets
              key: MINIO_SECRET_KEY
        volumeMounts:
        - name: minio-data
          mountPath: /data
        resources:
          requests:
            memory: "512Mi"
            cpu: "250m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
        livenessProbe:
          httpGet:
            path: /minio/health/live
            port: 9000
          initialDelaySeconds: 30
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /minio/health/ready
            port: 9000
          initialDelaySeconds: 10
          periodSeconds: 10
  volumeClaimTemplates:
  - metadata:
      name: minio-data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 50Gi
---
apiVersion: v1
kind: Service
metadata:
  name: minio
  namespace: link-archive
spec:
  selector:
    app: minio
  ports:
  - name: api
    port: 9000
    targetPort: 9000
  - name: console
    port: 9001
    targetPort: 9001
  clusterIP: None
---
# MinIO Console 접근용 (로컬 개발)
apiVersion: v1
kind: Service
metadata:
  name: minio-nodeport
  namespace: link-archive
spec:
  type: NodePort
  selector:
    app: minio
  ports:
  - name: console
    port: 9001
    targetPort: 9001
    nodePort: 30901
```

---

## 8. API Server (Deployment)

```yaml
# api-server.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api-server
  namespace: link-archive
spec:
  replicas: 2
  selector:
    matchLabels:
      app: api-server
  template:
    metadata:
      labels:
        app: api-server
        version: v1
    spec:
      containers:
      - name: api
        image: link-archive/api:latest
        imagePullPolicy: IfNotPresent
        ports:
        - containerPort: 8080
          name: http
        envFrom:
        - configMapRef:
            name: app-config
        - secretRef:
            name: app-secrets
        resources:
          requests:
            memory: "256Mi"
            cpu: "250m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
        readinessProbe:
          httpGet:
            path: /ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
---
apiVersion: v1
kind: Service
metadata:
  name: api-server
  namespace: link-archive
spec:
  selector:
    app: api-server
  ports:
  - port: 80
    targetPort: 8080
    name: http
  type: ClusterIP
```

---

## 9. Worker (Deployment)

```yaml
# worker.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: worker
  namespace: link-archive
spec:
  replicas: 3
  selector:
    matchLabels:
      app: worker
  template:
    metadata:
      labels:
        app: worker
        version: v1
    spec:
      containers:
      - name: worker
        image: link-archive/worker:latest
        imagePullPolicy: IfNotPresent
        envFrom:
        - configMapRef:
            name: app-config
        - secretRef:
            name: app-secrets
        env:
        - name: WORKER_ID
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        resources:
          requests:
            memory: "512Mi"
            cpu: "500m"
          limits:
            memory: "1Gi"
            cpu: "1000m"
        livenessProbe:
          exec:
            command:
            - /healthcheck
          initialDelaySeconds: 30
          periodSeconds: 30
```

---

## 10. Ingress (nginx)

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: api-ingress
  namespace: link-archive
  annotations:
    nginx.ingress.kubernetes.io/rate-limit: "100"
    nginx.ingress.kubernetes.io/limit-rps: "10"
    nginx.ingress.kubernetes.io/ssl-redirect: "false"  # 로컬 개발용
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  ingressClassName: nginx
  rules:
  - host: api.linkarchive.local
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: api-server
            port:
              number: 80
```

**로컬 hosts 설정** (`/etc/hosts`):
```
127.0.0.1 api.linkarchive.local
```

---

## 11. HorizontalPodAutoscaler (선택사항)

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: api-server-hpa
  namespace: link-archive
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: api-server
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: worker-hpa
  namespace: link-archive
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: worker
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
```

---

## 배포 순서

### 1. Namespace 생성
```bash
kubectl apply -f namespace.yaml
```

### 2. ConfigMap & Secret 생성
```bash
kubectl apply -f configmap.yaml
kubectl apply -f secret.yaml
```

### 3. 데이터베이스 배포
```bash
kubectl apply -f postgresql.yaml
kubectl apply -f redis.yaml
kubectl apply -f rabbitmq.yaml
kubectl apply -f minio.yaml
```

### 4. 데이터베이스 준비 대기
```bash
kubectl wait --for=condition=ready pod -l app=postgresql -n link-archive --timeout=300s
kubectl wait --for=condition=ready pod -l app=redis -n link-archive --timeout=300s
kubectl wait --for=condition=ready pod -l app=rabbitmq -n link-archive --timeout=300s
kubectl wait --for=condition=ready pod -l app=minio -n link-archive --timeout=300s
```

### 5. 초기 설정 (MinIO Bucket 생성)
```bash
# MinIO에 접속해서 bucket 생성
kubectl port-forward svc/minio-nodeport 9001:9001 -n link-archive
# http://localhost:9001 접속
# Bucket: thumbnails, content-cache 생성
```

### 6. 애플리케이션 배포
```bash
kubectl apply -f api-server.yaml
kubectl apply -f worker.yaml
```

### 7. Ingress 배포
```bash
# nginx-ingress-controller 설치 (미설치 시)
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.1/deploy/static/provider/cloud/deploy.yaml

kubectl apply -f ingress.yaml
```

### 8. 확인
```bash
kubectl get all -n link-archive
kubectl logs -f deployment/api-server -n link-archive
kubectl logs -f deployment/worker -n link-archive
```

---

## 유용한 명령어

### Pod 로그 확인
```bash
kubectl logs -f <pod-name> -n link-archive
kubectl logs -f deployment/api-server -n link-archive
```

### Pod 접속
```bash
kubectl exec -it <pod-name> -n link-archive -- /bin/sh
```

### Port Forward
```bash
# API Server
kubectl port-forward svc/api-server 8080:80 -n link-archive

# PostgreSQL
kubectl port-forward svc/postgresql 5432:5432 -n link-archive

# RabbitMQ Management
kubectl port-forward svc/rabbitmq 15672:15672 -n link-archive

# MinIO Console
kubectl port-forward svc/minio 9001:9001 -n link-archive
```

### 리소스 모니터링
```bash
kubectl top pods -n link-archive
kubectl top nodes
```

### 전체 삭제
```bash
kubectl delete namespace link-archive
```