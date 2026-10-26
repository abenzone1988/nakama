$ErrorActionPreference = 'Stop'
docker pull --platform linux/amd64 postgres:16-bookworm
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL 镜像拉取失败' }
docker tag postgres:16-bookworm docker.sparkinfi.com/nakama/fruit-postgres:16-bookworm
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL 镜像标记失败' }
docker push docker.sparkinfi.com/nakama/fruit-postgres:16-bookworm
if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL 镜像推送失败' }
