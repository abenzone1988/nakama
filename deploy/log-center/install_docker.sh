#!/bin/bash
set -e

echo "==== 移除旧版本 Docker ===="
yum remove -y docker \
              docker-client \
              docker-client-latest \
              docker-common \
              docker-latest \
              docker-latest-logrotate \
              docker-logrotate \
              docker-engine || true

echo "==== 安装依赖包 ===="
yum install -y yum-utils device-mapper-persistent-data lvm2

echo "==== 使用阿里云 Docker 源 ===="
yum-config-manager --add-repo http://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo

echo "==== 安装 Docker CE ===="
yum install -y docker-ce docker-ce-cli containerd.io

echo "==== 启动 Docker 并设置开机自启 ===="
systemctl enable docker
systemctl start docker

echo "==== 配置国内镜像加速器（阿里云） ===="
mkdir -p /etc/docker
cat > /etc/docker/daemon.json <<EOF
{
  "registry-mirrors": [
    "https://registry.cn-hangzhou.aliyuncs.com",
    "https://mirror.ccs.tencentyun.com",
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com"
  ]
}
EOF

systemctl daemon-reload
systemctl restart docker

echo "==== 验证 Docker 安装 ===="
docker --version

echo "==== 安装 Docker Compose v2（从 Gitee 镜像下载） ===="
COMPOSE_VERSION="v2.29.7"
curl -L "https://gitee.com/mirrors/docker-compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-x86_64" -o /usr/local/bin/docker-compose || {
  echo "Gitee 下载失败，尝试使用阿里云镜像..."
  curl -L "https://mirrors.aliyun.com/docker-toolbox/linux/compose/${COMPOSE_VERSION}/docker-compose-linux-x86_64" -o /usr/local/bin/docker-compose
}

chmod +x /usr/local/bin/docker-compose
ln -sf /usr/local/bin/docker-compose /usr/bin/docker-compose

echo "==== 验证 Docker Compose 安装 ===="
docker compose version

echo "==== Docker & Compose 安装完成！ ===="

