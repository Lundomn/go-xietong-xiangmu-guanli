# Go 协同项目管理系统 Helm 部署

这个 Chart 将当前 Docker Compose 部署转换为 Kubernetes 工作负载，默认包含：

- Vue/Nginx 前端
- Go API 网关
- `project-user` 用户服务
- `project-project` 项目任务服务
- 单副本 MySQL、Redis、etcd
- ConfigMap、Secret、健康探针、PVC、Ingress
- 可选 HPA（依赖 metrics-server 和资源 requests）

## 1. 构建镜像

Go 服务镜像依赖 `target` 目录下的 Linux 二进制，先执行：

```bash
GOOS=linux GOARCH=amd64 ./scripts/build.sh
```

然后构建并推送镜像。下面的仓库地址只是示例，请替换成自己的镜像仓库：

```bash
REGISTRY=ghcr.io/lundomn

docker build -f deploy/docker/project-api.Dockerfile -t ${REGISTRY}/go-xietong-api:latest .
docker build -f deploy/docker/project-user.Dockerfile -t ${REGISTRY}/go-xietong-user:latest .
docker build -f deploy/docker/project-project.Dockerfile -t ${REGISTRY}/go-xietong-project:latest .
docker build -f deploy/docker/frontend.Dockerfile -t ${REGISTRY}/go-xietong-frontend:latest .
docker build -f deploy/docker/mysql.Dockerfile -t ${REGISTRY}/go-xietong-mysql:8.0.42 .

docker push ${REGISTRY}/go-xietong-api:latest
docker push ${REGISTRY}/go-xietong-user:latest
docker push ${REGISTRY}/go-xietong-project:latest
docker push ${REGISTRY}/go-xietong-frontend:latest
docker push ${REGISTRY}/go-xietong-mysql:8.0.42
```

## 2. 安装 Chart

生产环境不要把真实密码直接写入 Git 或命令历史。这里用命令行展示流程，实际可使用 `secrets.existingSecret`：

```bash
helm upgrade --install go-xietong ./deploy/helm/go-xietong \
  --namespace go-xietong --create-namespace \
  --set images.api.repository=ghcr.io/lundomn/go-xietong-api \
  --set images.project.repository=ghcr.io/lundomn/go-xietong-project \
  --set images.user.repository=ghcr.io/lundomn/go-xietong-user \
  --set images.frontend.repository=ghcr.io/lundomn/go-xietong-frontend \
  --set images.mysql.repository=ghcr.io/lundomn/go-xietong-mysql \
  --set secrets.mysqlRootPassword='change-this-root-password' \
  --set secrets.mysqlPassword='change-this-root-password' \
  --set secrets.jwtAccessSecret='change-this-access-secret' \
  --set secrets.jwtRefreshSecret='change-this-refresh-secret'
```

前端镜像中的 Nginx 默认将 API 转发到名为 `api` 的 Kubernetes Service，Chart 已保留这个 Service 名称。不要只修改 `api.service.name`，除非同时重新构建前端 Nginx 配置。

## 3. 访问

默认不开 Ingress，可以端口转发：

```bash
kubectl port-forward -n go-xietong svc/go-xietong-frontend 8080:80
open http://127.0.0.1:8080
```

启用 Ingress：

```bash
helm upgrade --install go-xietong ./deploy/helm/go-xietong \
  --namespace go-xietong \
  --set ingress.enabled=true \
  --set ingress.host=project.example.com
```

## 4. 使用外部依赖

生产环境建议把 MySQL、Redis、etcd 换成托管服务或独立高可用集群：

```bash
--set mysql.enabled=false \
--set external.mysql.host=mysql.example.internal \
--set redis.enabled=false \
--set external.redis.host=redis.example.internal \
--set etcd.enabled=false \
--set external.etcd.host=etcd.example.internal
```

## 5. 文件上传说明

默认创建一个 `ReadWriteMany` 的上传 PVC，并挂载到 API 的 `/app/upload`。单节点集群使用本地 StorageClass 时，可能没有 RWX 能力；多副本生产环境应使用支持 RWX 的存储，或者将文件上传改造成 COS/OSS/MinIO 对象存储。

## 6. 开启 API 自动扩缩容

先给 API 配置 CPU requests，并确保集群安装 metrics-server：

下面命令用于已有 release 的升级；首次安装时还需要同时提供镜像和 Secret 参数。

```bash
helm upgrade --install go-xietong ./deploy/helm/go-xietong \
  --namespace go-xietong \
  --set autoscaling.api.enabled=true \
  --set api.resources.requests.cpu=200m \
  --set api.resources.requests.memory=256Mi \
  --set api.resources.limits.cpu=1 \
  --set api.resources.limits.memory=512Mi
```

## 7. 校验

```bash
helm lint ./deploy/helm/go-xietong \
  --set secrets.mysqlRootPassword=test-root \
  --set secrets.mysqlPassword=test-root \
  --set secrets.jwtAccessSecret=test-access \
  --set secrets.jwtRefreshSecret=test-refresh

helm template go-xietong ./deploy/helm/go-xietong \
  --namespace go-xietong \
  --set secrets.mysqlRootPassword=test-root \
  --set secrets.mysqlPassword=test-root \
  --set secrets.jwtAccessSecret=test-access \
  --set secrets.jwtRefreshSecret=test-refresh \
  | kubectl apply --dry-run=client -f -
```
