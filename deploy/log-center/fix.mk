cd /opt/log-center    # 若你目录仍在 /root/log-center，请先移动到 /opt/log-center 并同步修改 compose 路径
mkdir -p ./loki-data/{chunks,index,cache,compactor,wal}
sudo chown -R 10001:10001 ./loki-data
sudo chmod -R 0775 ./loki-data
# 启用 SELinux 时：
sudo chcon -R -t container_file_t ./loki-data || true

docker compose up -d loki
docker logs loki --tail=100
curl -sS http://127.0.0.1:3100/ready