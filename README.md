# Apple HME Manager

Apple 隐藏邮箱 (Hide My Email) 管理工具 - Go 后端 + React 前端

## 功能

- 🔐 Apple ID SRP 协议安全登录
- 🔑 支持双重认证 (2FA)
- 📧 查看/创建/删除 HME 隐藏邮箱
- 📦 批量创建 HME
- 💾 Session 自动管理

## 技术栈

**后端**
- Go 1.24
- Gin HTTP Framework
- SRP Protocol (Apple GSA)

**前端**
- React 19
- TypeScript
- TailwindCSS
- TanStack Query
- Zustand

## 快速开始

### 1. 安装依赖

```bash
# 后端
cd backend
go mod tidy

# 前端
cd frontend
npm install
```

### 2. 启动开发环境

```bash
# 启动后端 (使用 Air 热重载, 端口 8080)
cd backend
air

# 或者不用热重载
# go run .

# 启动前端 (端口 5173)
cd frontend
npm run dev
```

> **提示**: 首次使用需安装 Air: `go install github.com/air-verse/air@latest`

### 3. 访问应用

打开浏览器访问 http://localhost:5173

## 生产部署

```bash
# 构建前端
cd frontend
npm run build

# 启动后端 (自动托管前端静态文件)
cd backend
go build -o apple-hme-manager
./apple-hme-manager --port 8080
```

## API 接口

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /api/auth/login | SRP 登录 |
| POST | /api/auth/2fa | 2FA 验证 |
| POST | /api/auth/logout | 退出登录 |
| GET | /api/hme | 获取 HME 列表 |
| POST | /api/hme | 创建单个 HME |
| POST | /api/hme/batch | 批量创建 HME |
| DELETE | /api/hme/:id | 删除 HME |

## 注意事项

⚠️ 此工具仅供学习研究使用，请遵守 Apple 服务条款。

## License

MIT
