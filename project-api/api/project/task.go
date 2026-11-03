package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/jinzhu/copier"
	"go.uber.org/zap"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"test.com/project-api/pkg/model"
	"test.com/project-api/pkg/model/pro"
	"test.com/project-api/pkg/model/tasks"
	common "test.com/project-common"
	"test.com/project-common/errs"
	"test.com/project-common/tms"
	"test.com/project-grpc/task"
	"time"
)

type HandlerTask struct {
}

func (t *HandlerTask) taskStages(c *gin.Context) {
	result := &common.Result{}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	//1.获取参数 校验参数的合法性
	projectCode := c.PostForm("projectCode")
	page := &model.Page{}
	page.Bind(c)
	//2.调用grpc服务
	msg := &task.TaskReqMessage{
		MemberId:    c.GetInt64("memberId"),
		ProjectCode: projectCode,
		Page:        page.Page,
		PageSize:    page.PageSize,
	}
	stages, err := TaskServiceClient.TaskStages(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if stages == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	//3.处理响应
	var list []*tasks.TaskStagesResp
	copier.Copy(&list, stages.List)
	if list == nil {
		list = []*tasks.TaskStagesResp{}
	}
	for _, v := range list {
		if v == nil {
			continue
		}
		v.TasksLoading = true  //任务加载状态
		v.FixedCreator = false //添加任务按钮定位
		v.ShowTaskCard = false //是否显示创建卡片
		v.Tasks = []int{}
		v.DoneTasks = []int{}
		v.UnDoneTasks = []int{}
	}
	c.JSON(http.StatusOK, result.Success(gin.H{
		"list":  list,
		"total": stages.Total,
		"page":  page.Page,
	}))
}

func (t *HandlerTask) memberProjectList(c *gin.Context) {
	result := &common.Result{}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	//1.获取参数 校验参数的合法性
	projectCode := c.PostForm("projectCode")
	page := &model.Page{}
	page.Bind(c)
	//2.调用grpc服务
	msg := &task.TaskReqMessage{
		MemberId:    c.GetInt64("memberId"),
		ProjectCode: projectCode,
		Page:        page.Page,
		PageSize:    page.PageSize,
	}
	resp, err := TaskServiceClient.MemberProjectList(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if resp == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}

	var list []*pro.MemberProjectResp
	copier.Copy(&list, resp.List)
	if list == nil {
		list = []*pro.MemberProjectResp{}
	}
	c.JSON(http.StatusOK, result.Success(gin.H{
		"list":  list,
		"total": resp.Total,
		"page":  page.Page,
	}))

}

func (t *HandlerTask) taskList(c *gin.Context) {
	result := &common.Result{}
	stageCode := c.PostForm("stageCode")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	list, err := TaskServiceClient.TaskList(ctx, &task.TaskReqMessage{StageCode: stageCode, MemberId: c.GetInt64("memberId")})
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if list == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	var taskDisplayList []*tasks.TaskDisplay
	copier.Copy(&taskDisplayList, list.List)
	if taskDisplayList == nil {
		taskDisplayList = []*tasks.TaskDisplay{}
	}
	//返回给前端的数据 一定不要是null
	for _, v := range taskDisplayList {
		if v == nil {
			continue
		}
		if v.Tags == nil {
			v.Tags = []int{}
		}
		if v.ChildCount == nil {
			v.ChildCount = []int{}
		}
	}
	c.JSON(http.StatusOK, result.Success(taskDisplayList))
}

func (t *HandlerTask) saveTask(c *gin.Context) {
	result := &common.Result{}
	var req tasks.TaskSaveReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "参数格式有误"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		ProjectCode: req.ProjectCode,
		Name:        req.Name,
		StageCode:   req.StageCode,
		AssignTo:    req.AssignTo,
		MemberId:    c.GetInt64("memberId"),
	}
	taskMessage, err := TaskServiceClient.SaveTask(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if taskMessage == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	td := &tasks.TaskDisplay{}
	copier.Copy(td, taskMessage)
	if td != nil {
		if td.Tags == nil {
			td.Tags = []int{}
		}
		if td.ChildCount == nil {
			td.ChildCount = []int{}
		}
	}
	c.JSON(http.StatusOK, result.Success(td))
}

func (t *HandlerTask) taskSort(c *gin.Context) {
	result := &common.Result{}
	var req tasks.TaskSortReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "参数格式有误"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		PreTaskCode:  req.PreTaskCode,
		NextTaskCode: req.NextTaskCode,
		ToStageCode:  req.ToStageCode,
		MemberId:     c.GetInt64("memberId"),
	}
	_, err := TaskServiceClient.TaskSort(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	c.JSON(http.StatusOK, result.Success([]int{}))
}

func (t *HandlerTask) myTaskList(c *gin.Context) {
	result := &common.Result{}
	var req tasks.MyTaskReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "参数格式有误"))
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	memberId := c.GetInt64("memberId")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		MemberId: memberId,
		TaskType: int32(req.TaskType),
		Type:     int32(req.Type),
		Page:     req.Page,
		PageSize: req.PageSize,
	}
	myTaskListResponse, err := TaskServiceClient.MyTaskList(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if myTaskListResponse == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	var myTaskList []*tasks.MyTaskDisplay
	copier.Copy(&myTaskList, myTaskListResponse.List)
	if myTaskList == nil {
		myTaskList = []*tasks.MyTaskDisplay{}
	}
	for _, v := range myTaskList {
		if v == nil {
			continue
		}
		v.ProjectInfo = tasks.ProjectInfo{
			Name: v.ProjectName,
			Code: v.ProjectCode,
		}
	}
	c.JSON(http.StatusOK, result.Success(gin.H{
		"list":  myTaskList,
		"total": myTaskListResponse.Total,
	}))
}

func (t *HandlerTask) readTask(c *gin.Context) {
	result := &common.Result{}
	taskCode := c.PostForm("taskCode")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		TaskCode: taskCode,
		MemberId: c.GetInt64("memberId"),
	}
	taskMessage, err := TaskServiceClient.ReadTask(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if taskMessage == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	td := &tasks.TaskDisplay{}
	copier.Copy(td, taskMessage)
	if td != nil {
		if td.Tags == nil {
			td.Tags = []int{}
		}
		if td.ChildCount == nil {
			td.ChildCount = []int{}
		}
	}
	c.JSON(200, result.Success(td))
}

func (t *HandlerTask) taskDone(c *gin.Context) {
	result := &common.Result{}
	var req tasks.TaskDoneReq
	if err := c.ShouldBind(&req); err != nil || req.TaskCode == "" || (req.Done != 0 && req.Done != 1) {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "任务完成状态参数有误"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	taskMessage, err := TaskServiceClient.TaskDone(ctx, &task.TaskReqMessage{
		TaskCode: req.TaskCode,
		Done:     int32(req.Done),
		MemberId: c.GetInt64("memberId"),
	})
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if taskMessage == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	td := &tasks.TaskDisplay{}
	if err := copier.Copy(td, taskMessage); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务响应格式错误"))
		return
	}
	if td.Tags == nil {
		td.Tags = []int{}
	}
	if td.ChildCount == nil {
		td.ChildCount = []int{}
	}
	c.JSON(http.StatusOK, result.Success(td))
}

func (t *HandlerTask) editTask(c *gin.Context) {
	result := &common.Result{}
	taskCode := c.PostForm("taskCode")
	if taskCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "任务编号不能为空"))
		return
	}
	// The legacy frontend sends one field at a time. Whitelist the fields here
	// before passing the update to the task service.
	fields := []string{"name", "description", "pri", "status", "begin_time", "end_time", "work_time", "like", "star", "private"}
	field := ""
	value := ""
	for _, candidate := range fields {
		if candidateValue, ok := c.GetPostForm(candidate); ok {
			field = candidate
			value = candidateValue
			break
		}
	}
	if field == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "没有可修改的任务字段"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	taskMessage, err := TaskServiceClient.TaskEdit(ctx, &task.TaskReqMessage{
		TaskCode:  taskCode,
		MemberId:  c.GetInt64("memberId"),
		EditField: field,
		EditValue: value,
	})
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if taskMessage == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	td := &tasks.TaskDisplay{}
	if err := copier.Copy(td, taskMessage); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务响应格式错误"))
		return
	}
	if td.Tags == nil {
		td.Tags = []int{}
	}
	if td.ChildCount == nil {
		td.ChildCount = []int{}
	}
	c.JSON(http.StatusOK, result.Success(td))
}

func (t *HandlerTask) assignTask(c *gin.Context) {
	result := &common.Result{}
	taskCode := c.PostForm("taskCode")
	if taskCode == "" {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "任务编号不能为空"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	taskMessage, err := TaskServiceClient.TaskAssign(ctx, &task.TaskReqMessage{
		TaskCode: taskCode,
		AssignTo: c.PostForm("executorCode"),
		MemberId: c.GetInt64("memberId"),
	})
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if taskMessage == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	td := &tasks.TaskDisplay{}
	if err := copier.Copy(td, taskMessage); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务响应格式错误"))
		return
	}
	if td.Tags == nil {
		td.Tags = []int{}
	}
	if td.ChildCount == nil {
		td.ChildCount = []int{}
	}
	c.JSON(http.StatusOK, result.Success(td))
}

func (t *HandlerTask) listTaskMember(c *gin.Context) {
	result := &common.Result{}
	taskCode := c.PostForm("taskCode")
	page := &model.Page{}
	page.Bind(c)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		TaskCode: taskCode,
		MemberId: c.GetInt64("memberId"),
		Page:     page.Page,
		PageSize: page.PageSize,
	}
	taskMemberResponse, err := TaskServiceClient.ListTaskMember(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if taskMemberResponse == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	var tms []*tasks.TaskMember
	copier.Copy(&tms, taskMemberResponse.List)
	if tms == nil {
		tms = []*tasks.TaskMember{}
	}
	c.JSON(http.StatusOK, result.Success(gin.H{
		"list":  tms,
		"total": taskMemberResponse.Total,
		"page":  page.Page,
	}))
}

func (t *HandlerTask) taskLog(c *gin.Context) {
	result := &common.Result{}
	var req model.TaskLogReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "参数格式有误"))
		return
	}
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 10
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		TaskCode: req.TaskCode,
		MemberId: c.GetInt64("memberId"),
		Page:     int64(req.Page),
		PageSize: int64(req.PageSize),
		All:      int32(req.All),
		Comment:  int32(req.Comment),
	}
	taskLogResponse, err := TaskServiceClient.TaskLog(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if taskLogResponse == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	var tms []*model.ProjectLogDisplay
	copier.Copy(&tms, taskLogResponse.List)
	if tms == nil {
		tms = []*model.ProjectLogDisplay{}
	}
	c.JSON(http.StatusOK, result.Success(gin.H{
		"list":  tms,
		"total": taskLogResponse.Total,
		"page":  req.Page,
	}))
}

func (t *HandlerTask) taskWorkTimeList(c *gin.Context) {
	taskCode := c.PostForm("taskCode")
	result := &common.Result{}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		TaskCode: taskCode,
		MemberId: c.GetInt64("memberId"),
	}
	taskWorkTimeResponse, err := TaskServiceClient.TaskWorkTimeList(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if taskWorkTimeResponse == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	var tms []*model.TaskWorkTime
	copier.Copy(&tms, taskWorkTimeResponse.List)
	if tms == nil {
		tms = []*model.TaskWorkTime{}
	}
	for _, item := range tms {
		if item != nil {
			item.TaskCode = taskCode
		}
	}
	c.JSON(http.StatusOK, result.Success(tms))
}

func (t *HandlerTask) saveTaskWorkTime(c *gin.Context) {
	result := &common.Result{}
	var req model.SaveTaskWorkTimeReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "参数格式有误"))
		return
	}
	beginTime := tms.ParseTime(req.BeginTime)
	if beginTime <= 0 {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "beginTime格式不正确"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		TaskCode:  req.TaskCode,
		MemberId:  c.GetInt64("memberId"),
		Content:   req.Content,
		Num:       int32(req.Num),
		BeginTime: beginTime,
	}
	_, err := TaskServiceClient.SaveTaskWorkTime(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	c.JSON(http.StatusOK, result.Success([]int{}))
}

/*
func (t *HandlerTask) uploadFilesLegacy(c *gin.Context) {
	result := &common.Result{}
	req := model.UploadFileReq{}
	c.ShouldBind(&req)
	//处理文件
	multipartForm, _ := c.MultipartForm()
	file := multipartForm.File
	//假设只上传一个文件
	uploadFile := file["file"][0]
	//第一种 没有达成分片的条件
	key := ""
	if req.TotalChunks == 1 {
		//不分片
		path := "upload/" + req.ProjectCode + "/" + req.TaskCode + "/" + tms.FormatYMD(time.Now())
		if !fs.IsExist(path) {
			os.MkdirAll(path, os.ModePerm)
		}
		dst := path + "/" + req.Filename
		key = dst
		err := c.SaveUploadedFile(uploadFile, dst)
		if err != nil {
			c.JSON(http.StatusOK, result.Fail(-999, err.Error()))
			return
		}
	}
	if req.TotalChunks > 1 {
		//分片上传 无非就是先把每次的存储起来 追加就可以了
		path := "upload/" + req.ProjectCode + "/" + req.TaskCode + "/" + tms.FormatYMD(time.Now())
		if !fs.IsExist(path) {
			os.MkdirAll(path, os.ModePerm)
		}
		fileName := path + "/" + req.Identifier
		openFile, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_RDWR, os.ModePerm)
		if err != nil {
			c.JSON(http.StatusOK, result.Fail(-999, err.Error()))
			return
		}
		open, err := uploadFile.Open()
		if err != nil {
			c.JSON(http.StatusOK, result.Fail(-999, err.Error()))
			return
		}
		defer open.Close()
		buf := make([]byte, req.CurrentChunkSize)
		open.Read(buf)
		openFile.Write(buf)
		openFile.Close()
		key = fileName
		if req.TotalChunks == req.ChunkNumber {
			//最后一个分片了
			newPath := path + "/" + req.Filename
			key = newPath
			os.Rename(fileName, newPath)
		}
	}
	//调用服务 存入file表
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	fileUrl := "http://localhost/" + key
	msg := &task.TaskFileReqMessage{
		TaskCode:         req.TaskCode,
		ProjectCode:      req.ProjectCode,
		OrganizationCode: c.GetString("organizationCode"),
		PathName:         key,
		FileName:         req.Filename,
		Size:             int64(req.TotalSize),
		Extension:        path.Ext(key),
		FileUrl:          fileUrl,
		FileType:         file["file"][0].Header.Get("Content-Type"),
		MemberId:         c.GetInt64("memberId"),
	}
	if req.TotalChunks == req.ChunkNumber {
		_, err := TaskServiceClient.SaveTaskFile(ctx, msg)
		if err != nil {
			code, msg := errs.ParseGrpcError(err)
			c.JSON(http.StatusOK, result.Fail(code, msg))
		}
	}

	c.JSON(http.StatusOK, result.Success(gin.H{
		"file":        key,
		"hash":        "",
		"key":         key,
		"url":         "http://localhost/" + key,
		"projectName": req.ProjectName,
	}))
}
*/

const maxUploadSize int64 = 100 << 20

const maxUploadChunks = 4096

type uploadLock struct {
	mu   sync.Mutex
	refs int
}

var uploadLocks = struct {
	sync.Mutex
	items map[string]*uploadLock
}{items: make(map[string]*uploadLock)}

func acquireUploadLock(key string) func() {
	uploadLocks.Lock()
	lock := uploadLocks.items[key]
	if lock == nil {
		lock = &uploadLock{}
		uploadLocks.items[key] = lock
	}
	lock.refs++
	uploadLocks.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		uploadLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(uploadLocks.items, key)
		}
		uploadLocks.Unlock()
	}
}

func safeUploadPart(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 255 || filepath.Base(value) != value ||
		strings.ContainsAny(value, `/\\:`) {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validateUploadRequest(req model.UploadFileReq) error {
	if req.TotalChunks < 1 || req.ChunkNumber < 1 || req.ChunkNumber > req.TotalChunks {
		return errors.New("分片参数无效")
	}
	if req.TotalChunks > maxUploadChunks {
		return errors.New("分片数量超过限制")
	}
	if !safeUploadPart(req.ProjectCode) || !safeUploadPart(req.TaskCode) {
		return errors.New("项目或任务编号无效")
	}
	if req.TotalSize < 0 || int64(req.TotalSize) > maxUploadSize {
		return errors.New("文件大小超过限制")
	}
	if req.TotalChunks > 1 && !safeUploadPart(req.Identifier) {
		return errors.New("分片标识无效")
	}
	return nil
}

func uploadPartPath(uploadDir, identifier string, number int) string {
	// Keep client-controlled chunk names outside the published-file namespace.
	return filepath.Join(uploadDir, ".parts", identifier, strconv.Itoa(number)+".part")
}

var errUploadContractConflict = errors.New("upload parameters do not match the saved upload")
var errUploadPartConflict = errors.New("upload chunk does not match the saved content")
var errUploadSizeMismatch = errors.New("upload size does not match the saved contract")

type uploadContract struct {
	Version          int
	Filename         string
	TotalChunks      int
	TotalSize        int
	MemberID         int64
	OrganizationCode string
}

// Bind the active upload before replacing any chunk. A restart or a different
// authorized member must not reinterpret parts using different parameters.
func bindUploadContract(partDir string, want uploadContract) error {
	manifest := filepath.Join(partDir, "manifest.json")
	read := func() error {
		body, err := os.ReadFile(manifest)
		if err != nil {
			return err
		}
		var saved uploadContract
		if err := json.Unmarshal(body, &saved); err != nil {
			return err
		}
		if saved != want {
			return errUploadContractConflict
		}
		return nil
	}
	if err := read(); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries, err := os.ReadDir(partDir)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		// Parts without an original contract cannot safely be adopted.
		return errUploadContractConflict
	}
	body, err := json.Marshal(want)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(manifest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return read()
	}
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

// Publish only a completely written chunk. Retries may reuse the same bytes,
// but cannot truncate or replace an already accepted chunk.
// The bool reports whether this call created the accepted part.
func storeUploadPart(partPath string, input io.Reader, expectedSize int64) (bool, error) {
	output, err := os.CreateTemp(filepath.Dir(partPath), ".incoming-*")
	if err != nil {
		return false, err
	}
	temporaryPath := output.Name()
	defer func() {
		_ = output.Close()
		_ = os.Remove(temporaryPath)
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(output, hash), input)
	if err != nil {
		return false, err
	}
	if expectedSize >= 0 && written != expectedSize {
		return false, errors.New("uploaded chunk size does not match request")
	}
	if err := output.Sync(); err != nil {
		return false, err
	}
	if err := output.Close(); err != nil {
		return false, err
	}
	if err := os.Link(temporaryPath, partPath); err == nil {
		return true, nil
	} else if !errors.Is(err, os.ErrExist) {
		return false, err
	}
	info, err := os.Lstat(partPath)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() != written {
		return false, errUploadPartConflict
	}
	existing, err := os.Open(partPath)
	if err != nil {
		return false, err
	}
	existingHash := sha256.New()
	_, readErr := io.Copy(existingHash, existing)
	closeErr := existing.Close()
	if readErr != nil {
		return false, readErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	if !bytes.Equal(hash.Sum(nil), existingHash.Sum(nil)) {
		return false, errUploadPartConflict
	}
	return false, nil
}

func mergeUploadParts(uploadDir, identifier, filename string, totalChunks int, expectedSize int64) (string, error) {
	finalPath := filepath.Join(uploadDir, filename)
	assembled, err := os.CreateTemp(uploadDir, ".assembling-*")
	if err != nil {
		return "", err
	}
	assemblingPath := assembled.Name()

	cleanup := func() {
		_ = assembled.Close()
		_ = os.Remove(assemblingPath)
	}
	var totalSize int64
	for i := 1; i <= totalChunks; i++ {
		part := uploadPartPath(uploadDir, identifier, i)
		inputPart, openErr := os.Open(part)
		if openErr != nil {
			cleanup()
			return "", openErr
		}
		written, copyErr := io.Copy(assembled, inputPart)
		closeErr := inputPart.Close()
		if copyErr != nil || closeErr != nil {
			cleanup()
			if copyErr != nil {
				return "", copyErr
			}
			return "", closeErr
		}
		totalSize += written
	}
	// Validate before publication and before deleting any source part. A later
	// handler check cannot recover inputs already removed by a successful merge.
	if expectedSize > 0 && totalSize != expectedSize {
		cleanup()
		return "", errUploadSizeMismatch
	}
	if err = assembled.Close(); err != nil {
		_ = os.Remove(assemblingPath)
		return "", err
	}
	// Link publishes the complete file only if the destination does not exist.
	// Rename would silently replace an older attachment at the same path.
	if err = os.Link(assemblingPath, finalPath); err != nil {
		_ = os.Remove(assemblingPath)
		return "", err
	}
	_ = os.Remove(assemblingPath)
	// Keep every source part until the complete file is published. A later
	// read, close or publication failure must not destroy the input needed to retry.
	// Once publication succeeds, leftover parts are only cleanup work; they
	// must not turn an already-published file into a reported merge failure.
	for i := 1; i <= totalChunks; i++ {
		_ = os.Remove(uploadPartPath(uploadDir, identifier, i))
	}
	_ = os.Remove(filepath.Join(filepath.Dir(uploadPartPath(uploadDir, identifier, 1)), "manifest.json"))
	_ = os.Remove(filepath.Dir(uploadPartPath(uploadDir, identifier, 1)))
	return finalPath, nil
}

func (t *HandlerTask) downloadFile(c *gin.Context) {
	result := &common.Result{}
	relative := strings.TrimPrefix(filepath.ToSlash(c.Param("filepath")), "/")
	parts := strings.Split(relative, "/")
	if len(parts) != 4 || !safeUploadPart(parts[0]) || !safeUploadPart(parts[1]) ||
		!safeUploadPart(parts[2]) || !safeUploadPart(parts[3]) {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件路径无效"))
		return
	}
	if _, err := time.Parse("2006-01-02", parts[2]); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件日期无效"))
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	taskInfo, err := TaskServiceClient.ReadTask(ctx, &task.TaskReqMessage{
		TaskCode: parts[1],
		MemberId: c.GetInt64("memberId"),
	})
	if err != nil {
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if taskInfo == nil || taskInfo.ProjectCode != parts[0] {
		c.JSON(http.StatusNotFound, result.Fail(http.StatusNotFound, "文件不存在"))
		return
	}

	filePath := filepath.Clean(filepath.Join("upload", filepath.FromSlash(relative)))
	uploadRoot := filepath.Clean("upload")
	if filePath != uploadRoot && !strings.HasPrefix(filePath, uploadRoot+string(os.PathSeparator)) {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件路径无效"))
		return
	}
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		c.JSON(http.StatusNotFound, result.Fail(http.StatusNotFound, "文件不存在"))
		return
	}
	c.File(filePath)
}

func uploadURL(c *gin.Context, key string) string {
	relative := strings.TrimPrefix(filepath.ToSlash(key), "upload/")
	// The browser and the API are served from the same origin in both the
	// production Nginx setup and the Vue CLI development proxy. A relative URL
	// avoids generating an unusable container hostname or losing the frontend
	// port when the request passes through a reverse proxy.
	return "/project/upload/" + relative
}

func (t *HandlerTask) uploadFiles(c *gin.Context) {
	result := &common.Result{}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadSize+(1<<20))
	var req model.UploadFileReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "上传参数格式有误"))
		return
	}
	if err := validateUploadRequest(req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, err.Error()))
		return
	}

	// 在解析 multipart 和写入磁盘前校验任务权限，避免未授权请求利用上传接口
	// 向任意已知项目目录写入大文件。
	authCtx, authCancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	taskInfo, authErr := TaskServiceClient.ReadTask(authCtx, &task.TaskReqMessage{
		TaskCode: req.TaskCode,
		MemberId: c.GetInt64("memberId"),
	})
	authCancel()
	if authErr != nil {
		code, message := errs.ParseGrpcError(authErr)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}
	if taskInfo == nil || taskInfo.ProjectCode != req.ProjectCode {
		c.JSON(http.StatusNotFound, result.Fail(http.StatusNotFound, "任务不存在"))
		return
	}

	multipartForm, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "无法读取上传文件"))
		return
	}
	fileHeaders := multipartForm.File["file"]
	if len(fileHeaders) == 0 || fileHeaders[0] == nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "请选择要上传的文件"))
		return
	}
	uploadFile := fileHeaders[0]
	if uploadFile.Size > maxUploadSize {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件大小超过限制"))
		return
	}
	filename := req.Filename
	if filename == "" {
		filename = uploadFile.Filename
	}
	if !safeUploadPart(filename) {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件名无效"))
		return
	}

	uploadDir := filepath.Join("upload", req.ProjectCode, req.TaskCode, tms.FormatYMD(time.Now()))
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "创建上传目录失败"))
		return
	}
	key := ""
	if req.TotalChunks == 1 {
		key = filepath.Join(uploadDir, filename)
		input, err := uploadFile.Open()
		if err != nil {
			c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "打开上传文件失败"))
			return
		}
		output, err := os.OpenFile(key, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			_ = input.Close()
			if errors.Is(err, os.ErrExist) {
				c.JSON(http.StatusConflict, result.Fail(http.StatusConflict, "文件已存在，请修改文件名"))
				return
			}
			c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "保存上传文件失败"))
			return
		}
		written, copyErr := io.Copy(output, input)
		inputCloseErr, outputCloseErr := input.Close(), output.Close()
		if copyErr != nil || inputCloseErr != nil || outputCloseErr != nil || (uploadFile.Size >= 0 && written != uploadFile.Size) {
			_ = os.Remove(key) // This request created the destination exclusively.
			c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "写入上传文件失败"))
			return
		}
	} else {
		releaseUploadLock := acquireUploadLock(filepath.Join(req.ProjectCode, req.TaskCode, req.Identifier))
		defer releaseUploadLock()
		partPath := uploadPartPath(uploadDir, req.Identifier, req.ChunkNumber)
		if err := os.MkdirAll(filepath.Dir(partPath), 0755); err != nil {
			c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "创建分片目录失败"))
			return
		}
		contract := uploadContract{Version: 1, Filename: filename, TotalChunks: req.TotalChunks, TotalSize: req.TotalSize,
			MemberID: c.GetInt64("memberId"), OrganizationCode: c.GetString("organizationCode")}
		if err := bindUploadContract(filepath.Dir(partPath), contract); err != nil {
			if errors.Is(err, errUploadContractConflict) {
				c.JSON(http.StatusConflict, result.Fail(http.StatusConflict, "上传信息已变化，请重新发起上传"))
			} else {
				c.JSON(http.StatusInternalServerError, result.Fail(http.StatusInternalServerError, "无法核对原上传信息"))
			}
			return
		}
		input, err := uploadFile.Open()
		if err != nil {
			c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "打开上传文件失败"))
			return
		}
		partCreated, storeErr := storeUploadPart(partPath, input, uploadFile.Size)
		inputCloseErr := input.Close()
		if errors.Is(storeErr, errUploadPartConflict) {
			c.JSON(http.StatusConflict, result.Fail(http.StatusConflict, "分片内容已变化，请重新发起上传"))
			return
		}
		if storeErr != nil || inputCloseErr != nil {
			c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "写入分片失败"))
			return
		}
		key = partPath

		maxAllowedSize := maxUploadSize
		if req.TotalSize > 0 {
			maxAllowedSize = int64(req.TotalSize)
		}
		var totalPartSize int64

		// The last arriving chunk is not necessarily the chunk with the largest
		// number. Wait until every part exists, then let whichever request arrives
		// last perform the merge. This makes out-of-order uploads work without
		// forcing the client to retry the final chunk after an avoidable 400.
		allPartsReady := true
		for i := 1; i <= req.TotalChunks; i++ {
			part := uploadPartPath(uploadDir, req.Identifier, i)
			partInfo, statErr := os.Stat(part)
			if statErr != nil {
				if os.IsNotExist(statErr) {
					allPartsReady = false
					continue // Later-numbered parts still count toward the limit.
				}
				c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "检查分片失败"))
				return
			}
			totalPartSize += partInfo.Size()
			if totalPartSize > maxAllowedSize {
				if partCreated {
					_ = os.Remove(partPath)
				}
				c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "分片总大小超过限制"))
				return
			}
		}
		if !allPartsReady {
			partialURL := uploadURL(c, key)
			c.JSON(http.StatusOK, result.Success(gin.H{
				"file":        key,
				"hash":        "",
				"key":         key,
				"url":         partialURL,
				"projectName": req.ProjectName,
			}))
			return
		}

		finalPath, mergeErr := mergeUploadParts(uploadDir, req.Identifier, filename, req.TotalChunks, int64(req.TotalSize))
		if mergeErr != nil {
			if errors.Is(mergeErr, errUploadSizeMismatch) {
				c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件大小与分片声明不一致"))
			} else if errors.Is(mergeErr, os.ErrExist) {
				c.JSON(http.StatusConflict, result.Fail(http.StatusConflict, "文件已存在，请修改文件名"))
			} else if errors.Is(mergeErr, os.ErrNotExist) {
				c.JSON(http.StatusOK, result.Fail(http.StatusBadRequest, "分片尚未上传完整"))
			} else {
				c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "合并分片失败"))
			}
			return
		}
		key = finalPath
	}

	fileInfo, err := os.Stat(key)
	if err != nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusInternalServerError, "读取上传文件失败"))
		return
	}
	if req.TotalSize > 0 && fileInfo.Size() != int64(req.TotalSize) {
		_ = os.Remove(key)
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "文件大小与分片声明不一致"))
		return
	}
	totalSize := int64(req.TotalSize)
	if totalSize == 0 {
		totalSize = fileInfo.Size()
	}
	filePath := filepath.ToSlash(key)
	fileURL := uploadURL(c, filePath)
	msg := &task.TaskFileReqMessage{
		TaskCode:         req.TaskCode,
		ProjectCode:      req.ProjectCode,
		OrganizationCode: c.GetString("organizationCode"),
		PathName:         filePath,
		FileName:         filename,
		Size:             totalSize,
		Extension:        path.Ext(filename),
		FileUrl:          fileURL,
		FileType:         uploadFile.Header.Get("Content-Type"),
		MemberId:         c.GetInt64("memberId"),
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if _, err = TaskServiceClient.SaveTaskFile(ctx, msg); err != nil {
		// RPC 超时或断线不能证明服务端未提交附件关联。保留文件和核对线索，
		// 不在结果未知时做破坏性补偿，也不自动重复创建附件记录。
		zap.L().Warn("upload metadata result unresolved; preserving file",
			zap.String("taskCode", req.TaskCode), zap.String("uploadPath", filePath), zap.Error(err))
		code, message := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, message))
		return
	}

	c.JSON(http.StatusOK, result.Success(gin.H{
		"file":        filePath,
		"hash":        "",
		"key":         filePath,
		"url":         fileURL,
		"projectName": req.ProjectName,
	}))
}

func (t *HandlerTask) taskSources(c *gin.Context) {
	result := &common.Result{}
	taskCode := c.PostForm("taskCode")
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	sources, err := TaskServiceClient.TaskSources(ctx, &task.TaskReqMessage{TaskCode: taskCode, MemberId: c.GetInt64("memberId")})
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	if sources == nil {
		c.JSON(http.StatusOK, result.Fail(http.StatusBadGateway, "任务服务返回为空"))
		return
	}
	var slList []*model.SourceLink
	copier.Copy(&slList, sources.List)
	if slList == nil {
		slList = []*model.SourceLink{}
	}
	for _, item := range slList {
		if item != nil {
			item.SourceDetail.FileUrl = normalizeAttachmentURL(item.SourceDetail.FileUrl)
		}
	}
	c.JSON(http.StatusOK, result.Success(slList))
}

func (t *HandlerTask) createComment(c *gin.Context) {
	result := &common.Result{}
	req := model.CommentReq{}
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, result.Fail(http.StatusBadRequest, "参数格式有误"))
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	msg := &task.TaskReqMessage{
		TaskCode:       req.TaskCode,
		CommentContent: req.Comment,
		Mentions:       req.Mentions,
		MemberId:       c.GetInt64("memberId"),
	}
	_, err := TaskServiceClient.CreateComment(ctx, msg)
	if err != nil {
		code, msg := errs.ParseGrpcError(err)
		c.JSON(http.StatusOK, result.Fail(code, msg))
		return
	}
	c.JSON(http.StatusOK, result.Success(true))
}

func NewTask() *HandlerTask {
	return &HandlerTask{}
}
