package code_gen

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"os"
	"strconv"
	"strings"
	"text/template"
)

func connectMysql() *gorm.DB {
	// Code generation is an explicit developer tool. Read its connection from
	// environment variables instead of keeping credentials in source or
	// printing a password-bearing DSN to the test output.
	username := envOrDefault("CODEGEN_MYSQL_USER", "root")
	password := os.Getenv("CODEGEN_MYSQL_PASSWORD")
	host := envOrDefault("CODEGEN_MYSQL_HOST", "127.0.0.1")
	port := envIntOrDefault("CODEGEN_MYSQL_PORT", 3309)
	dbName := envOrDefault("CODEGEN_MYSQL_DATABASE", "msproject")
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", username, password, host, port, dbName)
	var err error
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		panic("连接数据库失败, error=" + err.Error())
	}
	return db
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

type Result struct {
	Field string
	Type  string
}
type StructResult struct {
	StructName string
	Result     []*Result
}
type MessageResult struct {
	MessageName string
	Result      []*Result
}

func GenStruct(table string, structName string) {
	db := connectMysql()
	var results []*Result
	db.Raw(fmt.Sprintf("describe %s", table)).Scan(&results)
	for _, v := range results {
		v.Field = Name(v.Field)
		v.Type = getType(v.Type)
	}
	tmpl, err := template.ParseFiles("./struct.tpl")
	log.Println(err)
	sr := StructResult{StructName: structName, Result: results}
	tmpl.Execute(os.Stdout, sr)
}

func GenProtoMessage(table string, messageName string) {
	db := connectMysql()
	var results []*Result
	db.Raw(fmt.Sprintf("describe %s", table)).Scan(&results)
	for _, v := range results {
		v.Field = Name(v.Field)
		v.Type = getMessageType(v.Type)
	}
	var fm template.FuncMap = make(map[string]any)
	fm["Add"] = func(v int, add int) int {
		return v + add
	}
	t := template.New("message.tpl")
	t.Funcs(fm)
	tmpl, err := t.ParseFiles("./message.tpl")
	log.Println(err)
	sr := MessageResult{MessageName: messageName, Result: results}
	err = tmpl.Execute(os.Stdout, sr)
	log.Println(err)
}

func getMessageType(t string) string {
	if strings.Contains(t, "bigint") {
		return "int64"
	}
	if strings.Contains(t, "varchar") {
		return "string"
	}
	if strings.Contains(t, "text") {
		return "string"
	}
	if strings.Contains(t, "tinyint") {
		return "int32"
	}
	if strings.Contains(t, "int") &&
		!strings.Contains(t, "tinyint") &&
		!strings.Contains(t, "bigint") {
		return "int32"
	}
	if strings.Contains(t, "double") {
		return "double"
	}
	return ""
}

func getType(t string) string {
	if strings.Contains(t, "bigint") {
		return "int64"
	}
	if strings.Contains(t, "varchar") {
		return "string"
	}
	if strings.Contains(t, "text") {
		return "string"
	}
	if strings.Contains(t, "tinyint") {
		return "int"
	}
	if strings.Contains(t, "int") &&
		!strings.Contains(t, "tinyint") &&
		!strings.Contains(t, "bigint") {
		return "int"
	}
	if strings.Contains(t, "double") {
		return "float64"
	}
	return ""
}

func Name(name string) string {
	var names = name[:]
	isSkip := false
	var sb strings.Builder
	for index, value := range names {
		if index == 0 {
			s := names[:index+1]
			s = strings.ToUpper(s)
			sb.WriteString(s)
			continue
		}
		if isSkip {
			isSkip = false
			continue
		}
		//95 下划线  user_name
		if value == 95 {
			s := names[index+1 : index+2]
			s = strings.ToUpper(s)
			sb.WriteString(s)
			isSkip = true
			continue
		} else {
			s := names[index : index+1]
			sb.WriteString(s)
		}
	}
	return sb.String()
}
