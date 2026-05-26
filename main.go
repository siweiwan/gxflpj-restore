package main

import (
	"archive/zip"
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	// 无论成功或失败，最后保持窗口打开防止闪退
	defer func() {
		fmt.Println("\n【程序运行结束】按回车键退出...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}()

	// 1. 获取目标文件：优先使用拖拽的文件，否则自动搜索同目录下的 .sql/.zip 文件
	var targetFile string
	if len(os.Args) >= 2 {
		targetFile = os.Args[1]
		fmt.Printf("📂 使用拖拽的目标文件: %s\n", targetFile)
	} else {
		exeDir := filepath.Dir(os.Args[0])
		sqlFiles, _ := filepath.Glob(filepath.Join(exeDir, "*.sql"))
		zipFiles, _ := filepath.Glob(filepath.Join(exeDir, "*.zip"))
		candidates := append(sqlFiles, zipFiles...)

		if len(candidates) == 0 {
			fmt.Println("💡 未检测到拖拽文件，同目录下也未找到 .sql/.zip 文件。")
			fmt.Print("📂 请将 .sql 或 .zip 文件的完整路径粘贴/拖入到此窗口，按回车确认: ")
			input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			targetFile = strings.TrimSpace(input)
			if targetFile == "" {
				fmt.Println("❌ 未输入任何路径，程序退出。")
				return
			}
		} else if len(candidates) == 1 {
			targetFile = candidates[0]
			fmt.Printf("📂 自动检测到文件: %s\n", targetFile)
		} else {
			fmt.Println("📂 检测到多个候选文件，请输入序号选择：")
			for i, f := range candidates {
				fmt.Printf("  %d. %s\n", i+1, filepath.Base(f))
			}
			fmt.Print("请输入序号: ")
			var choice int
			fmt.Scanf("%d", &choice)
			if choice < 1 || choice > len(candidates) {
				fmt.Println("❌ 无效的选择")
				return
			}
			targetFile = candidates[choice-1]
			fmt.Printf("📂 已选择: %s\n", targetFile)
		}
	}

	// 2. 加载同目录下的 .env 文件
	exePath, _ := os.Executable()
	envPath := filepath.Join(filepath.Dir(exePath), ".env")

	// 加载环境变量，如果文件不存在会报错
	if err := godotenv.Load(envPath); err != nil {
		fmt.Printf("❌ 无法加载 .env 配置文件: %v\n", err)
		return
	}

	// 从环境变量中读取配置
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	mysqlPath := os.Getenv("MYSQL_PATH")

	// 如果没有配置 MYSQL_PATH，默认使用 "mysql"
	if mysqlPath == "" {
		mysqlPath = "mysql"
	}

	// 3. 导入前设置 MySQL 参数，允许函数创建
	fmt.Println("🔧 正在解除函数创建限制...")
	setCmd := exec.Command(mysqlPath, "-h", host, "-P", port, "-u", user, "-p"+password, "-e",
		"SET GLOBAL log_bin_trust_function_creators = 1;")
	setErr, _ := setCmd.CombinedOutput()
	if strings.Contains(string(setErr), "ERROR") {
		fmt.Printf("⚠️  设置失败（可忽略）: %s", string(setErr))
	} else {
		fmt.Println("✅ 设置成功")
	}

	// 4. 如果拖入的是 ZIP 文件，先自动解压出 SQL
	sqlPath := targetFile
	if strings.ToLower(filepath.Ext(targetFile)) == ".zip" {
		fmt.Println("📦 检测到 ZIP 压缩包，正在自动解压...")
		extractedSql, err := unzipSpecificSql(targetFile)
		if err != nil {
			fmt.Printf("❌ 解压失败: %v\n", err)
			return
		}
		sqlPath = extractedSql
		// 程序退出时自动清理临时解压出来的大 SQL 文件
		defer os.Remove(sqlPath)
		fmt.Printf("🔓 解压成功: %s\n", sqlPath)
	}

	// 5. 打开 SQL 文件准备作为命令的标准输入（流式管道）
	sqlFile, err := os.Open(sqlPath)
	if err != nil {
		fmt.Printf("❌ 无法打开 SQL 文件: %v\n", err)
		return
	}
	defer sqlFile.Close()

	// 6. 构建原生 mysql 导入命令
	cmdArgs := []string{
		"-h", host,
		"-P", port,
		"-u", user,
		"-p" + password,
		dbname,
	}

	cmd := exec.Command(mysqlPath, cmdArgs...)
	cmd.Stdin = sqlFile

	var errLog strings.Builder
	cmd.Stderr = &errLog

	fmt.Printf("🚀 正在通过命令行全力导入数据到 [%s:%s] -> 数据库: %s ...\n", host, port, dbname)
	fmt.Println("⏳ 数据量较大，界面可能会暂时静止，请耐心等待...")

	startTime := time.Now()

	// 运行状态滚动指示
	done := make(chan struct{})
	go func() {
		spin := []string{"|", "/", "-", "\\"}
		i := 0
		for {
			select {
			case <-done:
				return
			default:
				elapsed := time.Since(startTime).Truncate(time.Second)
				fmt.Printf("\r⏳ 已运行 %v %s", elapsed, spin[i%4])
				i++
				time.Sleep(200 * time.Millisecond)
			}
		}
	}()

	err = cmd.Run()
	close(done)
	fmt.Print("\r")

	// 恢复安全设置
	exec.Command(mysqlPath, "-h", host, "-P", port, "-u", user, "-p"+password, "-e",
		"SET GLOBAL log_bin_trust_function_creators = 0;").Run()

	stderrOutput := errLog.String()
	logPath := filepath.Join(filepath.Dir(exePath), "restore_errors.log")

	if err != nil {
		fmt.Printf("❌ 导入中断！\n错误详情: %v\n", err)
		if stderrOutput != "" {
			fmt.Printf("MySQL 输出:\n%s\n", stderrOutput)
			os.WriteFile(logPath, []byte(stderrOutput), 0644)
			fmt.Printf("📝 错误日志: %s\n", logPath)
		}
		return
	}

	// 过滤密码警告，检查是否有其他错误
	hasError := false
	for _, line := range strings.Split(stderrOutput, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.Contains(line, "Using a password on the command line interface can be insecure") {
			hasError = true
			break
		}
	}
	if hasError {
		fmt.Println("⚠️  导入完成，但存在以下错误：")
		fmt.Println(stderrOutput)
		os.WriteFile(logPath, []byte(stderrOutput), 0644)
		fmt.Printf("📝 完整错误日志已保存到: %s\n", logPath)
	} else {
		fmt.Printf("🎉 恭喜！数据顺利导入成功！\n")
	}
	fmt.Printf("⏱️ 总共耗时: %v\n", time.Since(startTime))
}

// 解压 ZIP 中的第一个 SQL 文件到临时目录
func unzipSpecificSql(zipPath string) (string, error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		if strings.ToLower(filepath.Ext(f.Name)) == ".sql" {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			// 创建一个临时的目标 SQL 文件
			tmpSqlPath := filepath.Join(filepath.Dir(zipPath), "~tmp_"+f.Name)
			dstFile, err := os.OpenFile(tmpSqlPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return "", err
			}
			defer dstFile.Close()

			if _, err := io.Copy(dstFile, rc); err != nil {
				return "", err
			}
			return tmpSqlPath, nil
		}
	}
	return "", fmt.Errorf("ZIP 包内没有找到任何 .sql 文件")
}
