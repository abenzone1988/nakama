param([string]$Registry = 'docker.sparkinfi.com')
$ErrorActionPreference = 'Stop'
$images = @(
    @{ Source = 'postgres:16-alpine'; Target = 'fruit-postgres:16-alpine' },
    @{ Source = 'nginx:stable-alpine'; Target = 'fruit-nginx:stable-alpine' },
    @{ Source = 'certbot/certbot:latest'; Target = 'fruit-certbot:latest' }
)
foreach ($image in $images) {
    $target = "$Registry/nakama/$($image.Target)"
    docker pull --platform linux/amd64 $image.Source
    if ($LASTEXITCODE -ne 0) { throw "拉取失败：$($image.Source)" }
    docker tag $image.Source $target
    if ($LASTEXITCODE -ne 0) { throw "标记失败：$target" }
    docker push $target
    if ($LASTEXITCODE -ne 0) { throw "推送失败：$target" }
}
