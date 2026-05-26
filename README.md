# gxflpj-restore

MySQL dump 数据还原工具，支持 `.sql` 和 `.zip` 文件，通过管道流式导入，适合大文件。

## 使用方式

三种方式任选其一：

- **拖拽**：将 `.sql` / `.zip` 文件拖到 `restore.exe` 图标上
- **同目录**：将文件放在 `restore.exe` 同目录下，双击运行自动识别
- **粘贴路径**：双击运行后在窗口中粘贴文件路径，按回车

## 配置

复制 `.env.example` 为 `.env`，修改数据库连接信息：

```env
DB_HOST=127.0.0.1
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_NAME=your_database
MYSQL_PATH=mysql
```

`MYSQL_PATH` 默认为 `mysql`（使用系统 PATH），也可填写绝对路径如 `C:\Program Files\MySQL\MySQL Server 8.1\bin\mysql.exe`。

## 构建

```bash
go build -o restore.exe main.go
```

将 `restore.exe` 和 `.env` 放在同一目录即可使用。
