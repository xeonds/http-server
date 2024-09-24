package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"
)

//go:embed index.html
var templates embed.FS

var (
	port        string
	rootDir     string
	username    string
	password    string
	uploadLimit int64
	logFilePath string
	authEnabled bool

	r *gin.Engine
)

func init() {
	flag.StringVar(&port, "port", "8080", "Port to run the server on, default 8080")
	flag.StringVar(&rootDir, "root", "./", "Root directory to serve files from, default current directory")
	flag.StringVar(&username, "auth", "", "Enable authentication with username:password")
	flag.Int64Var(&uploadLimit, "upload", 10<<20, "Upload limit size in bytes (0 to disable uploads), default 10MB")
	flag.StringVar(&logFilePath, "log", "", "Path to log file (empty to disable logging), default disabled")

	flag.Parse()

	if username != "" {
		authEnabled = true
		creds := strings.SplitN(username, ":", 2)
		if len(creds) == 2 {
			username = creds[0]
			password = creds[1]
		} else {
			fmt.Println("Invalid auth format. Use username:password")
			os.Exit(1)
		}
	}

	r = gin.Default()
	r.SetHTMLTemplate(template.Must(template.New("").ParseFS(templates, "index.html")))
	if authEnabled {
		r.Use(func(c *gin.Context) {
			user, pass, ok := c.Request.BasicAuth()
			if !ok || user != username || pass != password {
				c.Header("WWW-Authenticate", `Basic realm="Authorization Required"`)
				c.AbortWithStatus(http.StatusUnauthorized)
				return
			}
			c.Next()
		})
	}
	if logFilePath != "" {
		f, err := os.Create(logFilePath)
		if err != nil {
			fmt.Println("Failed to create log file:", err)
			os.Exit(1)
		}
		gin.DefaultWriter = f
		r.Use(gin.Logger())
	}
	r.GET("/*path", handleList)
	r.POST("/*path", handleUpload)
	r.DELETE("/*path", handleDelete)
}

type FileMetadata struct {
	Name      string
	Size      int64
	CreatedAt string
	IsDir     bool
}

func main() {
	r.Run(":" + port)
}

// Handlers
func handleList(c *gin.Context) {
	path := c.Param("path")
	if path == "" {
		path = "/"
	}
	dirPath := filepath.Join(rootDir, filepath.Clean(path))
	if info, err := os.Stat(dirPath); err == nil && !info.IsDir() {
		c.File(dirPath)
	} else {
		list, err := os.ReadDir(dirPath)
		if err != nil {
			c.String(http.StatusInternalServerError, "Error reading directory")
			return
		}

		var dirs, files []FileMetadata
		for _, file := range list {
			info, err := file.Info()
			if err != nil {
				c.String(http.StatusInternalServerError, "Error reading file info")
				return
			}

			if info.IsDir() {
				dirs = append(dirs, FileMetadata{
					Name:      info.Name(),
					IsDir:     true,
					CreatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
				})
			} else {
				files = append(files, FileMetadata{
					Name:      info.Name(),
					Size:      info.Size(),
					IsDir:     false,
					CreatedAt: info.ModTime().Format("2006-01-02 15:04:05"),
				})
			}
		}
		usedSpace, totalSpace := getDiskUsage()
		c.HTML(http.StatusOK, "index.html", gin.H{
			"Path":        path,
			"Dirs":        dirs,
			"Files":       files,
			"UsedSpace":   usedSpace,
			"TotalSpace":  totalSpace,
			"UploadLimit": uploadLimit,
		})
	}
}
func handleUpload(c *gin.Context) {
	path := c.Param("path")
	dirPath := filepath.Join(rootDir, filepath.Clean(path))
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadLimit)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.String(http.StatusBadRequest, "Failed to upload file: %v", err)
		return
	}
	defer file.Close()
	filePath := filepath.Join(dirPath, header.Filename)
	out, err := os.Create(filePath)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to save file: %v", err)
		return
	}
	defer out.Close()
	_, err = out.ReadFrom(file)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to save file: %v", err)
		return
	}
	c.String(http.StatusOK, "File uploaded successfully")
}
func handleDelete(c *gin.Context) {
	path := c.Param("path")
	filePath := filepath.Join(rootDir, filepath.Clean(path))
	err := os.Remove(filePath)
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to delete file: %v", err)
		return
	}
	c.String(http.StatusOK, "File deleted successfully")
}

// utils
func getDiskUsage() (string, string) {
	var stat syscall.Statfs_t
	syscall.Statfs(rootDir, &stat)
	total := stat.Blocks * uint64(stat.Bsize)
	used := (stat.Blocks - stat.Bfree) * uint64(stat.Bsize)

	return humanReadableBytes(used), humanReadableBytes(total)
}
func humanReadableBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
