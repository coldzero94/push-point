.PHONY: help minikube-start minikube-stop minikube-status k8s-up k8s-down k8s-restart k8s-status k8s-logs k8s-apply-infra k8s-apply-app k8s-delete-all pods logs-api logs-worker logs-redis logs-postgres port-forward-api port-forward-minio scale-api scale-worker hpa-status build-api build-worker docker-build deploy metrics

# 기본 변수
NAMESPACE := push-point
MINIKUBE_CPUS := 4
MINIKUBE_MEMORY := 6144
MINIKUBE_DRIVER := docker

## help: 사용 가능한 명령어 목록 표시
help:
	@echo "=== Push-Point Makefile 명령어 ==="
	@echo ""
	@echo "== Minikube 관리 =="
	@echo "  make minikube-start    - Minikube 클러스터 시작"
	@echo "  make minikube-stop     - Minikube 클러스터 중지"
	@echo "  make minikube-status   - Minikube 상태 확인"
	@echo "  make minikube-delete   - Minikube 클러스터 삭제"
	@echo ""
	@echo "== K8s 전체 관리 =="
	@echo "  make k8s-up            - 전체 인프라 + 앱 배포"
	@echo "  make k8s-down          - 전체 리소스 삭제"
	@echo "  make k8s-restart       - 전체 재시작 (삭제 후 재배포)"
	@echo "  make k8s-status        - 전체 리소스 상태 확인"
	@echo ""
	@echo "== K8s 개별 배포 =="
	@echo "  make k8s-apply-infra   - 인프라만 배포 (DB, Redis, MinIO)"
	@echo "  make k8s-apply-app     - 애플리케이션만 배포 (API, Worker)"
	@echo ""
	@echo "== 모니터링 =="
	@echo "  make pods              - Pod 목록 및 상태"
	@echo "  make logs-api          - API Server 로그"
	@echo "  make logs-worker       - Worker 로그"
	@echo "  make logs-redis        - Redis 로그"
	@echo "  make logs-postgres     - PostgreSQL 로그"
	@echo "  make hpa-status        - HPA 상태 확인"
	@echo "  make metrics           - 리소스 사용량 확인"
	@echo ""
	@echo "== Port Forward =="
	@echo "  make port-forward-api  - API Server 포트 포워드 (8080)"
	@echo "  make port-forward-minio - MinIO Console 포트 포워드 (9001)"
	@echo ""
	@echo "== 스케일링 =="
	@echo "  make scale-api REPLICAS=3  - API Server 스케일 조정"
	@echo "  make scale-worker REPLICAS=5 - Worker 스케일 조정"
	@echo ""
	@echo "== Docker 빌드 =="
	@echo "  make build-api         - API Server 이미지 빌드"
	@echo "  make build-worker      - Worker 이미지 빌드"
	@echo "  make docker-build      - 전체 이미지 빌드"
	@echo ""
	@echo "== 완전 배포 =="
	@echo "  make deploy            - 빌드 + Minikube + 전체 배포"
	@echo ""

## minikube-start: Minikube 클러스터 시작
minikube-start:
	@echo "🚀 Minikube 클러스터 시작 중..."
	minikube start --cpus=$(MINIKUBE_CPUS) --memory=$(MINIKUBE_MEMORY) --driver=$(MINIKUBE_DRIVER)
	@echo "✅ Minikube 시작 완료!"
	@echo "📊 클러스터 정보:"
	@kubectl cluster-info

## minikube-stop: Minikube 클러스터 중지
minikube-stop:
	@echo "🛑 Minikube 클러스터 중지 중..."
	minikube stop
	@echo "✅ Minikube 중지 완료!"

## minikube-status: Minikube 상태 확인
minikube-status:
	minikube status

## minikube-delete: Minikube 클러스터 삭제
minikube-delete:
	@echo "🗑️  Minikube 클러스터 삭제 중..."
	minikube delete
	@echo "✅ Minikube 삭제 완료!"

## k8s-apply-infra: 인프라 리소스 배포 (Namespace, ConfigMap, Secret, DB, Redis, MinIO)
k8s-apply-infra:
	@echo "📦 인프라 배포 중..."
	kubectl apply -f k8s/namespace.yaml
	kubectl apply -f k8s/configmap.yaml
	kubectl apply -f k8s/secret.yaml
	kubectl apply -f k8s/postgresql.yaml
	kubectl apply -f k8s/redis.yaml
	kubectl apply -f k8s/minio.yaml
	@echo "⏳ Pod가 Ready 상태가 될 때까지 대기 중..."
	kubectl wait --for=condition=ready pod -l app=postgresql -n $(NAMESPACE) --timeout=300s || true
	kubectl wait --for=condition=ready pod -l app=redis -n $(NAMESPACE) --timeout=300s || true
	kubectl wait --for=condition=ready pod -l app=minio -n $(NAMESPACE) --timeout=300s || true
	@echo "✅ 인프라 배포 완료!"

## k8s-apply-app: 애플리케이션 배포 (API Server, Worker, HPA)
k8s-apply-app:
	@echo "🚀 애플리케이션 배포 중..."
	kubectl apply -f k8s/api-server.yaml
	kubectl apply -f k8s/worker.yaml
	kubectl apply -f k8s/hpa.yaml
	@echo "✅ 애플리케이션 배포 완료!"

## k8s-up: 전체 K8s 리소스 배포
k8s-up: k8s-apply-infra k8s-apply-app
	@echo "🎉 전체 배포 완료!"
	@make k8s-status

## k8s-down: 전체 K8s 리소스 삭제
k8s-down:
	@echo "🗑️  K8s 리소스 삭제 중..."
	kubectl delete namespace $(NAMESPACE) --ignore-not-found=true
	@echo "✅ 리소스 삭제 완료!"

## k8s-restart: K8s 재시작 (삭제 후 재배포)
k8s-restart: k8s-down
	@echo "⏳ 5초 대기 중..."
	@sleep 5
	@make k8s-up

## k8s-status: 전체 리소스 상태 확인
k8s-status:
	@echo "📊 === K8s 리소스 상태 ==="
	@echo ""
	@echo "🔹 Namespace:"
	@kubectl get namespace $(NAMESPACE) 2>/dev/null || echo "  Namespace가 존재하지 않습니다"
	@echo ""
	@echo "🔹 Pods:"
	@kubectl get pods -n $(NAMESPACE) 2>/dev/null || echo "  Pod가 없습니다"
	@echo ""
	@echo "🔹 Services:"
	@kubectl get svc -n $(NAMESPACE) 2>/dev/null || echo "  Service가 없습니다"
	@echo ""
	@echo "🔹 Deployments:"
	@kubectl get deployments -n $(NAMESPACE) 2>/dev/null || echo "  Deployment가 없습니다"
	@echo ""
	@echo "🔹 HPA:"
	@kubectl get hpa -n $(NAMESPACE) 2>/dev/null || echo "  HPA가 없습니다"

## pods: Pod 목록 표시
pods:
	kubectl get pods -n $(NAMESPACE) -o wide

## logs-api: API Server 로그 확인
logs-api:
	kubectl logs -f -n $(NAMESPACE) -l app=api-server --tail=100

## logs-worker: Worker 로그 확인
logs-worker:
	kubectl logs -f -n $(NAMESPACE) -l app=worker --tail=100

## logs-redis: Redis 로그 확인
logs-redis:
	kubectl logs -f -n $(NAMESPACE) -l app=redis --tail=100

## logs-postgres: PostgreSQL 로그 확인
logs-postgres:
	kubectl logs -f -n $(NAMESPACE) -l app=postgresql --tail=100

## port-forward-api: API Server 포트 포워드
port-forward-api:
	@echo "🔗 API Server 포트 포워드: http://localhost:8080"
	kubectl port-forward -n $(NAMESPACE) svc/api-server 8080:80

## port-forward-minio: MinIO Console 포트 포워드
port-forward-minio:
	@echo "🔗 MinIO Console 포트 포워드: http://localhost:9001"
	@echo "   ID: minioadmin / PW: minioadmin123"
	kubectl port-forward -n $(NAMESPACE) svc/minio 9001:9001

## scale-api: API Server 스케일 조정
scale-api:
	@if [ -z "$(REPLICAS)" ]; then \
		echo "❌ REPLICAS 변수를 지정해주세요. 예: make scale-api REPLICAS=3"; \
		exit 1; \
	fi
	@echo "📈 API Server를 $(REPLICAS)개로 스케일 조정 중..."
	kubectl scale deployment api-server -n $(NAMESPACE) --replicas=$(REPLICAS)
	@echo "✅ 스케일 조정 완료!"

## scale-worker: Worker 스케일 조정
scale-worker:
	@if [ -z "$(REPLICAS)" ]; then \
		echo "❌ REPLICAS 변수를 지정해주세요. 예: make scale-worker REPLICAS=5"; \
		exit 1; \
	fi
	@echo "📈 Worker를 $(REPLICAS)개로 스케일 조정 중..."
	kubectl scale deployment worker -n $(NAMESPACE) --replicas=$(REPLICAS)
	@echo "✅ 스케일 조정 완료!"

## hpa-status: HPA 상태 확인
hpa-status:
	@echo "📊 === HPA 상태 ==="
	kubectl get hpa -n $(NAMESPACE)
	@echo ""
	@echo "📊 === API Server HPA 상세 ==="
	kubectl describe hpa api-server-hpa -n $(NAMESPACE) 2>/dev/null || echo "  HPA가 존재하지 않습니다"
	@echo ""
	@echo "📊 === Worker HPA 상세 ==="
	kubectl describe hpa worker-hpa -n $(NAMESPACE) 2>/dev/null || echo "  HPA가 존재하지 않습니다"

## metrics: 리소스 사용량 확인
metrics:
	@echo "📊 === 리소스 사용량 ==="
	@echo ""
	@echo "🔹 Pod 리소스 사용량:"
	kubectl top pods -n $(NAMESPACE) 2>/dev/null || echo "  Metrics Server가 활성화되어 있지 않습니다. 'minikube addons enable metrics-server' 실행"
	@echo ""
	@echo "🔹 Node 리소스 사용량:"
	kubectl top nodes 2>/dev/null || echo "  Metrics Server가 활성화되어 있지 않습니다"

## build-api: API Server Docker 이미지 빌드
build-api:
	@echo "🔨 API Server 이미지 빌드 중..."
	@eval $$(minikube docker-env) && \
	docker build -t push-point/api-server:latest -f backend/docker/Dockerfile.api backend/
	@echo "✅ API Server 이미지 빌드 완료!"

## build-worker: Worker Docker 이미지 빌드
build-worker:
	@echo "🔨 Worker 이미지 빌드 중..."
	@eval $$(minikube docker-env) && \
	docker build -t push-point/worker:latest -f backend/docker/Dockerfile.worker backend/
	@echo "✅ Worker 이미지 빌드 완료!"

## docker-build: 전체 Docker 이미지 빌드
docker-build: build-api build-worker
	@echo "✅ 전체 이미지 빌드 완료!"

## deploy: 완전 배포 (Minikube 시작 + 전체 배포)
deploy: minikube-start k8s-up
	@echo ""
	@echo "🎉 ============================================"
	@echo "✅ 배포 완료!"
	@echo "🎉 ============================================"
	@echo ""
	@echo "📍 접속 정보:"
	@echo "  - API Server: minikube service api-server-nodeport -n $(NAMESPACE)"
	@echo "  - MinIO Console: minikube service minio-nodeport -n $(NAMESPACE)"
	@echo ""
	@echo "🔍 유용한 명령어:"
	@echo "  - 상태 확인: make k8s-status"
	@echo "  - Pod 목록: make pods"
	@echo "  - API 로그: make logs-api"
	@echo "  - Worker 로그: make logs-worker"
	@echo ""

## k8s-delete-all: 모든 K8s 리소스 강제 삭제
k8s-delete-all:
	@echo "⚠️  모든 리소스를 강제로 삭제합니다..."
	kubectl delete all --all -n $(NAMESPACE) --force --grace-period=0 || true
	kubectl delete pvc --all -n $(NAMESPACE) --force --grace-period=0 || true
	kubectl delete namespace $(NAMESPACE) --force --grace-period=0 || true
	@echo "✅ 강제 삭제 완료!"
