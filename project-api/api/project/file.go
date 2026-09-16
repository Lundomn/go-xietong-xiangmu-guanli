package project

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"test.com/project-api/pkg/model"
	common "test.com/project-common"
	"test.com/project-common/encrypts"
	"test.com/project-common/errs"
	"test.com/project-grpc/task"
	"time"

	"github.com/gin-gonic/gin"
)

// normalizeAttachmentURL keeps old absolute file URLs usable after the
// frontend is served behind Nginx. New uploads already store relative paths.
func normalizeAttachmentURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil {
		path := parsed.Path
		if index := strings.Index(path, "/project/upload/"); index >= 0 {
			return path[index:] + querySuffix(parsed)
		}
		if index := strings.Index(path, "/upload/"); index >= 0 {
			return "/project" + path[index:] + querySuffix(parsed)
		}
	}
	if strings.HasPrefix(raw, "upload/") {
		return "/project/" + raw
	}
	if strings.HasPrefix(raw, "/upload/") {
		return "/project" + raw
	}
	return raw
}

func querySuffix(parsed *url.URL) string {
	if parsed == nil || parsed.RawQuery == "" {
		return ""
	}
	return "?" + parsed.RawQuery
}

func projectFileItem(source *task.TaskSourceMessage, memberID int64, memberName string) gin.H {
	if source == nil || source.SourceDetail == nil {
		return nil
	}
	detail := source.SourceDetail
	fullName := detail.GetFullName()
	if fullName == "" {
		fullName = detail.GetTitle()
	}
	if fullName == "" {
		fullName = "未命名文件"
	}
	title := detail.GetTitle()
	if title == "" {
		title = fullName
	}
	creatorName := "项目成员"
	if memberName != "" && detail.GetCreateBy() == encrypts.EncryptNoErr(memberID) {
		creatorName = memberName
	}
	return gin.H{
		"id":           detail.GetId(),
		"code":         detail.GetCode(),
		"title":        title,
		"fullName":     fullName,
		"extension":    detail.GetExtension(),
		"size":         detail.GetSize(),
		"file_url":     normalizeAttachmentURL(detail.GetFileUrl()),
		"file_type":    detail.GetFileType(),
		"create_time":  detail.GetCreateTime(),
		"creatorName":  creatorName,
		"deleted":      detail.GetDeleted(),
		"task_code":    detail.GetTaskCode(),
		"project_code": detail.GetProjectCode(),
		"source_code":  source.GetCode(),
	}
}

func (t *HandlerTask) projectFiles(c *gin.Context) {
	result := &common.Result{}
	projectCode := c.PostForm("projectCode")
	if projectCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "projectCode不能为空"))
		return
	}
	page := &model.Page{}
	page.Bind(c)
	if page.PageSize > 200 {
		page.PageSize = 200
	}
	deleted := 0
	if value, err := strconv.Atoi(c.PostForm("deleted")); err == nil && value == 1 {
		deleted = 1
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	items, err := collectProjectTasks(ctx, projectCode, c.GetInt64("memberId"))
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	memberName := c.GetString("memberName")
	files := make([]gin.H, 0)
	seen := make(map[string]struct{})
	for _, item := range items {
		if item == nil || item.Code == "" {
			continue
		}
		sources, sourceErr := TaskServiceClient.TaskSources(ctx, &task.TaskReqMessage{
			TaskCode: item.Code,
			MemberId: c.GetInt64("memberId"),
		})
		if sourceErr != nil {
			code, message := errs.ParseGrpcError(sourceErr)
			c.JSON(http.StatusOK, result.Fail(code, message))
			return
		}
		if sources == nil {
			continue
		}
		for _, source := range sources.List {
			file := projectFileItem(source, c.GetInt64("memberId"), memberName)
			if file == nil || file["deleted"].(int32) != int32(deleted) {
				continue
			}
			code, _ := file["code"].(string)
			if code == "" {
				continue
			}
			if _, exists := seen[code]; exists {
				continue
			}
			seen[code] = struct{}{}
			files = append(files, file)
		}
	}
	sort.SliceStable(files, func(i, j int) bool {
		left, _ := files[i]["create_time"].(string)
		right, _ := files[j]["create_time"].(string)
		return left > right
	})
	total := len(files)
	start := int((page.Page - 1) * page.PageSize)
	if start > total {
		start = total
	}
	end := start + int(page.PageSize)
	if end > total {
		end = total
	}
	list := files[start:end]
	c.JSON(http.StatusOK, result.Success(gin.H{
		"list":  list,
		"total": total,
		"page":  page.Page,
	}))
}

func (t *HandlerTask) doFileAction(c *gin.Context, action string) {
	result := &common.Result{}
	fileCode := c.PostForm("fileCode")
	if fileCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件编号不能为空"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	response, err := TaskServiceClient.FileAction(ctx, &task.TaskReqMessage{
		FileCode:   fileCode,
		FileTitle:  c.PostForm("title"),
		FileAction: action,
		MemberId:   c.GetInt64("memberId"),
	})
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if action == "delete" && response != nil && response.SourceDetail != nil {
		// Metadata is owned by the project service while the actual upload is
		// stored in the API container's shared upload volume. Best-effort cleanup
		// prevents permanent deletion from leaving an orphaned disk file.
		_ = removeUploadedFile(response.SourceDetail.PathName)
	}
	c.JSON(http.StatusOK, result.Success([]int{}))
}

func removeUploadedFile(pathName string) error {
	relative := filepath.ToSlash(strings.TrimSpace(pathName))
	if !strings.HasPrefix(relative, "upload/") {
		return os.ErrInvalid
	}
	target := filepath.Clean(filepath.Join("upload", filepath.FromSlash(strings.TrimPrefix(relative, "upload/"))))
	root := filepath.Clean("upload")
	if target == root || !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return os.ErrInvalid
	}
	err := os.Remove(target)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (t *HandlerTask) editFile(c *gin.Context) {
	t.doFileAction(c, "edit")
}

func (t *HandlerTask) recycleFile(c *gin.Context) {
	t.doFileAction(c, "recycle")
}

func (t *HandlerTask) recoveryFile(c *gin.Context) {
	t.doFileAction(c, "recovery")
}

func (t *HandlerTask) deleteFile(c *gin.Context) {
	t.doFileAction(c, "delete")
}
