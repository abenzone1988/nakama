# 添加公告功能

## 1. 数据结构设计
- [ ] 设计Announcement数据结构
  ```go
  type Announcement struct {
    Id      string    `json:"id"`
    Title   string    `json:"title"`
    Content string    `json:"content"`
    Time    time.Time `json:"time"`
    Img     string    `json:"img"`
    Status  int       `json:"status"` // 0:草稿 1:已发布 2:已下线
  }
  ```

## 2. 后端实现
- [ ] 数据库表结构
  - 创建announcements表
  - 添加必要的索引
- [ ] API接口实现
  - POST /v2/console/announcement - 创建公告
  - PUT /v2/console/announcement/{id} - 更新公告
  - DELETE /v2/console/announcement/{id} - 删除公告
  - GET /v2/console/announcement/{id} - 获取单个公告
  - GET /v2/console/announcements - 获取公告列表(分页)
- [ ] 图片上传功能
  - 参考@pub_addressable.py实现CDN上传
  - 实现图片上传接口
  - 处理图片URL生成

## 3. 前端实现
- [ ] 管理界面组件(console/ui/src/views/admin/announcement.vue)
  - 公告列表组件
    - 分页显示
    - 状态过滤
    - 搜索功能
  - 新增/编辑表单
    - 标题、内容输入
    - 图片上传
    - 状态选择
  - 预览功能
- [ ] API服务
  - 实现与后端API的交互方法
  - 处理响应和错误
- [ ] 路由配置
  - 添加公告管理路由
  - 配置访问权限

## 4. 客户端实现
- [ ] API调用
  - 实现获取公告列表接口
  - 处理响应数据
- [ ] 界面展示
  - 公告列表页面
  - 公告详情页面
  - 图片加载和缓存处理

## 5. 测试计划
- [ ] 单元测试
  - 后端API测试
  - 数据库操作测试
- [ ] 集成测试
  - API端到端测试
  - 前端组件测试
- [ ] 性能测试
  - 列表加载性能
  - 图片加载性能

## 6. 部署计划
- [ ] 数据库迁移脚本
- [ ] 配置更新
- [ ] 部署文档更新
