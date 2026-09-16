package task_service_v1

import (
	"context"
	"github.com/jinzhu/copier"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"test.com/project-common/encrypts"
	"test.com/project-common/errs"
	"test.com/project-common/tms"
	"test.com/project-grpc/task"
	"test.com/project-grpc/user/login"
	"test.com/project-project/internal/dao"
	"test.com/project-project/internal/data"
	"test.com/project-project/internal/database"
	"test.com/project-project/internal/database/tran"
	"test.com/project-project/internal/domain"
	"test.com/project-project/internal/repo"
	"test.com/project-project/internal/rpc"
	"test.com/project-project/pkg/model"
	"time"
)

type TaskService struct {
	task.UnimplementedTaskServiceServer
	cache                  repo.Cache
	transaction            tran.Transaction
	projectRepo            repo.ProjectRepo
	projectTemplateRepo    repo.ProjectTemplateRepo
	taskStagesTemplateRepo repo.TaskStagesTemplateRepo
	taskStagesRepo         repo.TaskStagesRepo
	taskRepo               repo.TaskRepo
	projectLogRepo         repo.ProjectLogRepo
	taskWorkTimeRepo       repo.TaskWorkTimeRepo
	fileRepo               repo.FileRepo
	sourceLinkRepo         repo.SourceLinkRepo
	taskWorkTimeDomain     *domain.TaskWorkTimeDomain
}

func New() *TaskService {
	return &TaskService{
		cache:                  dao.Rc,
		transaction:            dao.NewTransaction(),
		projectRepo:            dao.NewProjectDao(),
		projectTemplateRepo:    dao.NewProjectTemplateDao(),
		taskStagesTemplateRepo: dao.NewTaskStagesTemplateDao(),
		taskStagesRepo:         dao.NewTaskStagesDao(),
		taskRepo:               dao.NewTaskDao(),
		projectLogRepo:         dao.NewProjectLogDao(),
		taskWorkTimeRepo:       dao.NewTaskWorkTimeDao(),
		fileRepo:               dao.NewFileDao(),
		sourceLinkRepo:         dao.NewSourceLinkDao(),
		taskWorkTimeDomain:     domain.NewTaskWorkTimeDomain(),
	}
}

func (t *TaskService) requireProjectMember(ctx context.Context, projectCode, memberID int64) error {
	if projectCode <= 0 || memberID <= 0 {
		return errs.GrpcError(model.InvalidParameter)
	}
	project, err := t.projectRepo.FindProjectById(ctx, projectCode)
	if err != nil {
		return errs.GrpcError(model.DBError)
	}
	if project == nil {
		return errs.GrpcError(model.InvalidParameter)
	}
	if project.Deleted == model.Deleted {
		return errs.GrpcError(model.ProjectAlreadyDeleted)
	}
	members, _, err := t.projectRepo.FindProjectMemberByPid(ctx, projectCode)
	if err != nil {
		return errs.GrpcError(model.DBError)
	}
	for _, member := range members {
		if member != nil && member.MemberCode == memberID {
			return nil
		}
	}
	return errs.GrpcError(model.InvalidParameter)
}

func (t *TaskService) requireTaskMember(ctx context.Context, taskCode, memberID int64) (*data.Task, error) {
	if taskCode <= 0 || memberID <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskInfo, err := t.taskRepo.FindTaskById(ctx, taskCode)
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if taskInfo == nil {
		return nil, errs.GrpcError(model.TaskNotFound)
	}
	if taskInfo.Deleted == model.Deleted {
		return nil, errs.GrpcError(model.TaskNotFound)
	}
	if err := t.requireProjectMember(ctx, taskInfo.ProjectCode, memberID); err != nil {
		return nil, err
	}
	if taskInfo.Private == 1 {
		taskMember, err := t.taskRepo.FindTaskMemberByTaskId(ctx, taskCode, memberID)
		if err != nil {
			return nil, errs.GrpcError(model.DBError)
		}
		if taskMember == nil {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
	}
	return taskInfo, nil
}

func (t *TaskService) TaskStages(co context.Context, msg *task.TaskReqMessage) (*task.TaskStagesResponse, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCode := encrypts.DecryptNoErr(msg.ProjectCode)
	if projectCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	page := msg.Page
	pageSize := msg.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	ctx, cancel := context.WithTimeout(co, 2*time.Second)
	defer cancel()
	if err := t.requireProjectMember(ctx, projectCode, msg.MemberId); err != nil {
		return nil, err
	}
	stages, total, err := t.taskStagesRepo.FindStagesByProjectId(ctx, projectCode, page, pageSize)
	if err != nil {
		zap.L().Error("project SaveProject taskStagesRepo.FindStagesByProjectId error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}

	var tsMessages []*task.TaskStagesMessage
	copier.Copy(&tsMessages, stages)
	if tsMessages == nil {
		return &task.TaskStagesResponse{List: tsMessages, Total: 0}, nil
	}
	stagesMap := data.ToTaskStagesMap(stages)
	for _, v := range tsMessages {
		if v == nil {
			continue
		}
		taskStages := stagesMap[int(v.Id)]
		if taskStages == nil {
			continue
		}
		v.Code = encrypts.EncryptNoErr(int64(v.Id))
		v.CreateTime = tms.FormatByMill(taskStages.CreateTime)
		v.ProjectCode = msg.ProjectCode
	}
	return &task.TaskStagesResponse{List: tsMessages, Total: total}, nil
}

func (t *TaskService) MemberProjectList(co context.Context, msg *task.TaskReqMessage) (*task.MemberProjectResponse, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//1. 去 project_member表 去查询 用户id列表
	ctx, cancel := context.WithTimeout(co, 2*time.Second)
	defer cancel()
	projectCode := encrypts.DecryptNoErr(msg.ProjectCode)
	if projectCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if err := t.requireProjectMember(ctx, projectCode, msg.MemberId); err != nil {
		return nil, err
	}
	projectMembers, total, err := t.projectRepo.FindProjectMemberByPid(ctx, projectCode)
	if err != nil {
		zap.L().Error("project MemberProjectList projectRepo.FindProjectMemberByPid error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	//2.拿上用户id列表 去请求用户信息
	if projectMembers == nil || len(projectMembers) <= 0 {
		return &task.MemberProjectResponse{List: nil, Total: 0}, nil
	}
	var mIds []int64
	pmMap := make(map[int64]*data.ProjectMember)
	for _, v := range projectMembers {
		if v == nil || v.MemberCode <= 0 {
			continue
		}
		mIds = append(mIds, v.MemberCode)
		pmMap[v.MemberCode] = v
	}
	//请求用户信息
	userMsg := &login.UserMessage{
		MIds: mIds,
	}
	memberMessageList, err := rpc.LoginServiceClient.FindMemInfoByIds(ctx, userMsg)
	if err != nil {
		zap.L().Error("project MemberProjectList LoginServiceClient.FindMemInfoByIds error", zap.Error(err))
		return nil, err
	}
	if memberMessageList == nil {
		return nil, errs.GrpcError(model.DBError)
	}
	var list []*task.MemberProjectMessage
	for _, v := range memberMessageList.List {
		if v == nil {
			continue
		}
		projectMember := pmMap[v.Id]
		if projectMember == nil {
			continue
		}
		owner := projectMember.IsOwner
		mpm := &task.MemberProjectMessage{
			MemberCode: v.Id,
			Name:       v.Name,
			Avatar:     v.Avatar,
			Email:      v.Email,
			Code:       v.Code,
		}
		if v.Id == owner {
			mpm.IsOwner = 1
		}
		list = append(list, mpm)
	}
	return &task.MemberProjectResponse{List: list, Total: total}, nil
}
func (t *TaskService) TaskList(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskListResponse, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	stageCode := encrypts.DecryptNoErr(msg.StageCode)
	if stageCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	stage, err := t.taskStagesRepo.FindById(c, int(stageCode))
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if stage == nil || stage.Deleted == model.Deleted {
		return nil, errs.GrpcError(model.TaskStagesNotNull)
	}
	if err := t.requireProjectMember(c, stage.ProjectCode, msg.MemberId); err != nil {
		return nil, err
	}
	taskList, err := t.taskRepo.FindTaskByStageCode(c, int(stageCode))
	if err != nil {
		zap.L().Error("project task TaskList FindTaskByStageCode error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	var taskDisplayList []*data.TaskDisplay
	var mIds []int64
	for _, v := range taskList {
		if v == nil {
			continue
		}
		display := v.ToTaskDisplay()
		if v.Private == 1 {
			//代表隐私模式
			taskMember, err := t.taskRepo.FindTaskMemberByTaskId(c, v.Id, msg.MemberId)
			if err != nil {
				zap.L().Error("project task TaskList taskRepo.FindTaskMemberByTaskId error", zap.Error(err))
				return nil, errs.GrpcError(model.DBError)
			}
			if taskMember != nil {
				display.CanRead = model.CanRead
			} else {
				display.CanRead = model.NoCanRead
			}
		}
		taskDisplayList = append(taskDisplayList, display)
		if v.AssignTo > 0 {
			mIds = append(mIds, v.AssignTo)
		}
	}
	if len(taskDisplayList) == 0 {
		return &task.TaskListResponse{List: []*task.TaskMessage{}}, nil
	}
	if len(mIds) == 0 {
		var taskMessageList []*task.TaskMessage
		copier.Copy(&taskMessageList, taskDisplayList)
		return &task.TaskListResponse{List: taskMessageList}, nil
	}
	// in ()
	messageList, err := rpc.LoginServiceClient.FindMemInfoByIds(ctx, &login.UserMessage{MIds: mIds})
	if err != nil {
		zap.L().Error("project task TaskList LoginServiceClient.FindMemInfoByIds error", zap.Error(err))
		return nil, err
	}
	if messageList == nil {
		return nil, errs.GrpcError(model.DBError)
	}
	memberMap := make(map[int64]*login.MemberMessage)
	for _, v := range messageList.List {
		if v == nil {
			continue
		}
		memberMap[v.Id] = v
	}
	for _, v := range taskDisplayList {
		message := memberMap[encrypts.DecryptNoErr(v.AssignTo)]
		if message == nil {
			continue
		}
		e := data.Executor{
			Name:   message.Name,
			Avatar: message.Avatar,
		}
		v.Executor = e
	}
	var taskMessageList []*task.TaskMessage
	copier.Copy(&taskMessageList, taskDisplayList)
	return &task.TaskListResponse{List: taskMessageList}, nil
}
func (t *TaskService) SaveTask(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskMessage, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//1. 检查业务逻辑
	if strings.TrimSpace(msg.Name) == "" {
		return nil, errs.GrpcError(model.TaskNameNotNull)
	}
	stageCode := encrypts.DecryptNoErr(msg.StageCode)
	if stageCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskStages, err := t.taskStagesRepo.FindById(ctx, int(stageCode))
	if err != nil {
		zap.L().Error("project task SaveTask taskStagesRepo.FindById error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if taskStages == nil {
		return nil, errs.GrpcError(model.TaskStagesNotNull)
	}
	projectCode := encrypts.DecryptNoErr(msg.ProjectCode)
	if projectCode <= 0 || int64(taskStages.ProjectCode) != projectCode {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	project, err := t.projectRepo.FindProjectById(ctx, projectCode)
	if err != nil {
		zap.L().Error("project task SaveTask projectRepo.FindProjectById error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if project == nil || project.Deleted == model.Deleted {
		return nil, errs.GrpcError(model.ProjectAlreadyDeleted)
	}
	maxIdNum, err := t.taskRepo.FindTaskMaxIdNum(ctx, projectCode)
	if err != nil {
		zap.L().Error("project task SaveTask taskRepo.FindTaskMaxIdNum error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if maxIdNum == nil {
		a := 0
		maxIdNum = &a
	}
	maxSort, err := t.taskRepo.FindTaskSort(ctx, projectCode, stageCode)
	if err != nil {
		zap.L().Error("project task SaveTask taskRepo.FindTaskSort error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if maxSort == nil {
		a := 0
		maxSort = &a
	}
	assignTo := msg.MemberId
	if msg.AssignTo != "" {
		assignTo = encrypts.DecryptNoErr(msg.AssignTo)
	}
	if assignTo <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectMembers, _, err := t.projectRepo.FindProjectMemberByPid(ctx, projectCode)
	if err != nil {
		zap.L().Error("project task SaveTask find project members error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	creatorInProject := false
	assigneeInProject := false
	for _, projectMember := range projectMembers {
		if projectMember == nil {
			continue
		}
		if projectMember.MemberCode == msg.MemberId {
			creatorInProject = true
		}
		if projectMember.MemberCode == assignTo {
			assigneeInProject = true
		}
	}
	if !creatorInProject || !assigneeInProject {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	ts := &data.Task{
		Name:        msg.Name,
		CreateTime:  time.Now().UnixMilli(),
		CreateBy:    msg.MemberId,
		AssignTo:    assignTo,
		ProjectCode: projectCode,
		StageCode:   int(stageCode),
		IdNum:       *maxIdNum + 1,
		Private:     project.OpenTaskPrivate,
		Sort:        *maxSort + 65536,
		BeginTime:   time.Now().UnixMilli(),
		EndTime:     time.Now().Add(2 * 24 * time.Hour).UnixMilli(),
	}
	err = t.transaction.Action(func(conn database.DbConn) error {
		err = t.taskRepo.SaveTask(ctx, conn, ts)
		if err != nil {
			zap.L().Error("project task SaveTask taskRepo.SaveTask error", zap.Error(err))
			return errs.GrpcError(model.DBError)
		}

		tm := &data.TaskMember{
			MemberCode: assignTo,
			TaskCode:   ts.Id,
			JoinTime:   time.Now().UnixMilli(),
			IsOwner:    model.Owner,
		}
		if assignTo == msg.MemberId {
			tm.IsExecutor = model.Executor
		}
		err = t.taskRepo.SaveTaskMember(ctx, conn, tm)
		if err != nil {
			zap.L().Error("project task SaveTask taskRepo.SaveTaskMember error", zap.Error(err))
			return errs.GrpcError(model.DBError)
		}
		return nil
	})

	if err != nil {
		return nil, err
	}
	display := ts.ToTaskDisplay()
	member, err := rpc.LoginServiceClient.FindMemInfoById(ctx, &login.UserMessage{MemId: assignTo})
	if err != nil || member == nil {
		zap.L().Warn("project task SaveTask member lookup failed after commit", zap.Error(err))
	} else {
		display.Executor = data.Executor{
			Name:   member.Name,
			Avatar: member.Avatar,
			Code:   member.Code,
		}
	}
	//添加任务动态
	if err := createProjectLog(ctx, t.projectLogRepo, ts.ProjectCode, ts.Id, ts.Name, ts.CreateBy, ts.AssignTo, "create", "task"); err != nil {
		zap.L().Warn("project task SaveTask create project log error", zap.Error(err))
	}

	tm := &task.TaskMessage{}
	copier.Copy(tm, display)
	return tm, nil
}

func createProjectLog(
	ctx context.Context,
	logRepo repo.ProjectLogRepo,
	projectCode int64,
	taskCode int64,
	taskName string,
	actorMemberCode int64,
	toMemberCode int64,
	logType string,
	actionType string) error {
	remark := ""
	if logType == "create" {
		remark = "创建了任务"
	}
	pl := &data.ProjectLog{
		MemberCode:  actorMemberCode,
		SourceCode:  taskCode,
		Content:     taskName,
		Remark:      remark,
		ProjectCode: projectCode,
		CreateTime:  time.Now().UnixMilli(),
		Type:        logType,
		ActionType:  actionType,
		Icon:        "plus",
		IsComment:   0,
		IsRobot:     0,
	}
	return logRepo.SaveProjectLog(ctx, pl)
}

func (t *TaskService) TaskSort(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskSortResponse, error) {
	if msg == nil || msg.PreTaskCode == "" || msg.ToStageCode == "" || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	preTaskCode := encrypts.DecryptNoErr(msg.PreTaskCode)
	toStageCode := encrypts.DecryptNoErr(msg.ToStageCode)
	if preTaskCode <= 0 || toStageCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if msg.NextTaskCode != "" && encrypts.DecryptNoErr(msg.NextTaskCode) <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	err := t.sortTask(ctx, msg.MemberId, preTaskCode, msg.NextTaskCode, toStageCode)
	if err != nil {
		return nil, err
	}
	return &task.TaskSortResponse{}, nil

}

func (t *TaskService) sortTask(ctx context.Context, memberID, preTaskCode int64, nextTaskCode string, toStageCode int64) error {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	movingTask, err := t.requireTaskMember(c, preTaskCode, memberID)
	if err != nil {
		zap.L().Error("project task TaskSort find moving task error", zap.Error(err))
		return err
	}
	toStage, err := t.taskStagesRepo.FindById(c, int(toStageCode))
	if err != nil {
		return errs.GrpcError(model.DBError)
	}
	if toStage == nil || toStage.Deleted == model.Deleted || toStage.ProjectCode != movingTask.ProjectCode {
		return errs.GrpcError(model.InvalidParameter)
	}
	nextID := encrypts.DecryptNoErr(nextTaskCode)
	if nextTaskCode != "" && nextID == movingTask.Id {
		if movingTask.StageCode != int(toStageCode) {
			return errs.GrpcError(model.InvalidParameter)
		}
		return nil
	}
	targetTasks, err := t.taskRepo.FindTaskByStageCode(c, int(toStageCode))
	if err != nil {
		zap.L().Error("project task TaskSort find target stage tasks error", zap.Error(err))
		return errs.GrpcError(model.DBError)
	}
	ordered := make([]*data.Task, 0, len(targetTasks)+1)
	insertAt := len(targetTasks)
	nextFound := nextTaskCode == ""
	for _, item := range targetTasks {
		if item == nil || item.Id == movingTask.Id {
			continue
		}
		if item.Id == nextID {
			insertAt = len(ordered)
			nextFound = true
		}
		ordered = append(ordered, item)
	}
	if !nextFound {
		return errs.GrpcError(model.TaskNotFound)
	}
	if insertAt > len(ordered) {
		insertAt = len(ordered)
	}
	ordered = append(ordered, nil)
	copy(ordered[insertAt+1:], ordered[insertAt:])
	ordered[insertAt] = movingTask

	var sourceTasks []*data.Task
	if movingTask.StageCode != int(toStageCode) {
		sourceTasks, err = t.taskRepo.FindTaskByStageCode(c, movingTask.StageCode)
		if err != nil {
			zap.L().Error("project task TaskSort find source stage tasks error", zap.Error(err))
			return errs.GrpcError(model.DBError)
		}
	}
	return t.transaction.Action(func(conn database.DbConn) error {
		const sortStep = 65536
		for index, item := range ordered {
			item.StageCode = int(toStageCode)
			item.Sort = (index + 1) * sortStep
			if err := t.taskRepo.UpdateTaskSort(c, conn, item); err != nil {
				zap.L().Error("project task TaskSort update target task error", zap.Error(err))
				return errs.GrpcError(model.DBError)
			}
		}
		sourceIndex := 0
		for _, item := range sourceTasks {
			if item == nil || item.Id == movingTask.Id {
				continue
			}
			sourceIndex++
			item.Sort = sourceIndex * sortStep
			if err := t.taskRepo.UpdateTaskSort(c, conn, item); err != nil {
				zap.L().Error("project task TaskSort update source task error", zap.Error(err))
				return errs.GrpcError(model.DBError)
			}
		}
		return nil
	})
}

func (t *TaskService) MyTaskList(ctx context.Context, msg *task.TaskReqMessage) (*task.MyTaskListResponse, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if msg.MemberId <= 0 || msg.TaskType < 1 || msg.TaskType > 3 || msg.Type < 0 || msg.Type > 1 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if msg.Page <= 0 {
		msg.Page = 1
	}
	if msg.PageSize <= 0 {
		msg.PageSize = 10
	}
	var tsList []*data.Task
	var err error
	var total int64
	if msg.TaskType == 1 {
		//我执行的
		tsList, total, err = t.taskRepo.FindTaskByAssignTo(ctx, msg.MemberId, int(msg.Type), msg.Page, msg.PageSize)
		if err != nil {
			zap.L().Error("project task MyTaskList taskRepo.FindTaskByAssignTo error", zap.Error(err))
			return nil, errs.GrpcError(model.DBError)
		}
	}
	if msg.TaskType == 2 {
		//我参与的
		tsList, total, err = t.taskRepo.FindTaskByMemberCode(ctx, msg.MemberId, int(msg.Type), msg.Page, msg.PageSize)
		if err != nil {
			zap.L().Error("project task MyTaskList taskRepo.FindTaskByMemberCode error", zap.Error(err))
			return nil, errs.GrpcError(model.DBError)
		}
	}
	if msg.TaskType == 3 {
		//我创建的
		tsList, total, err = t.taskRepo.FindTaskByCreateBy(ctx, msg.MemberId, int(msg.Type), msg.Page, msg.PageSize)
		if err != nil {
			zap.L().Error("project task MyTaskList taskRepo.FindTaskByCreateBy error", zap.Error(err))
			return nil, errs.GrpcError(model.DBError)
		}
	}
	if tsList == nil || len(tsList) <= 0 {
		return &task.MyTaskListResponse{List: nil, Total: 0}, nil
	}
	var pids []int64
	var mids []int64
	for _, v := range tsList {
		if v == nil {
			continue
		}
		pids = append(pids, v.ProjectCode)
		if v.AssignTo > 0 {
			mids = append(mids, v.AssignTo)
		}
	}
	pList, err := t.projectRepo.FindProjectByIds(ctx, pids)
	if err != nil {
		zap.L().Error("project task MyTaskList projectRepo.FindProjectByIds error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	projectMap := data.ToProjectMap(pList)
	mList, err := rpc.LoginServiceClient.FindMemInfoByIds(ctx, &login.UserMessage{MIds: mids})
	if err != nil {
		zap.L().Error("project task MyTaskList LoginServiceClient.FindMemInfoByIds error", zap.Error(err))
		return nil, err
	}
	if mList == nil {
		return nil, errs.GrpcError(model.DBError)
	}
	mMap := make(map[int64]*login.MemberMessage)
	for _, v := range mList.List {
		if v == nil {
			continue
		}
		mMap[v.Id] = v
	}
	var mtdList []*data.MyTaskDisplay
	for _, v := range tsList {
		if v == nil {
			continue
		}
		memberMessage := mMap[v.AssignTo]
		if projectMap[v.ProjectCode] == nil {
			return nil, errs.GrpcError(model.DBError)
		}
		name, avatar := "", ""
		if memberMessage != nil {
			name = memberMessage.Name
			avatar = memberMessage.Avatar
		}
		mtd := v.ToMyTaskDisplay(projectMap[v.ProjectCode], name, avatar)
		mtdList = append(mtdList, mtd)
	}
	var myMsgs []*task.MyTaskMessage
	copier.Copy(&myMsgs, mtdList)
	return &task.MyTaskListResponse{List: myMsgs, Total: total}, nil
}

func (t *TaskService) ReadTask(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskMessage, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//根据taskCode查询任务详情 根据任务查询项目详情 根据任务查询任务步骤详情 查询任务的执行者的成员详情
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	if taskCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	taskInfo, err := t.requireTaskMember(c, taskCode, msg.MemberId)
	if err != nil {
		zap.L().Error("project task ReadTask taskRepo FindTaskById error", zap.Error(err))
		return nil, err
	}
	display := taskInfo.ToTaskDisplay()
	if taskInfo.Private == 1 {
		//代表隐私模式
		taskMember, err := t.taskRepo.FindTaskMemberByTaskId(ctx, taskInfo.Id, msg.MemberId)
		if err != nil {
			zap.L().Error("project task TaskList taskRepo.FindTaskMemberByTaskId error", zap.Error(err))
			return nil, errs.GrpcError(model.DBError)
		}
		if taskMember != nil {
			display.CanRead = model.CanRead
		} else {
			display.CanRead = model.NoCanRead
		}
	}
	pj, err := t.projectRepo.FindProjectById(c, taskInfo.ProjectCode)
	if err != nil {
		zap.L().Error("project task ReadTask projectRepo.FindProjectById error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if pj == nil {
		return nil, errs.GrpcError(model.DBError)
	}
	display.ProjectName = pj.Name
	taskStages, err := t.taskStagesRepo.FindById(c, taskInfo.StageCode)
	if err != nil {
		zap.L().Error("project task ReadTask taskStagesRepo.FindById error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if taskStages == nil {
		return nil, errs.GrpcError(model.TaskStagesNotNull)
	}
	display.StageName = taskStages.Name
	if taskInfo.AssignTo > 0 {
		memberMessage, err := rpc.LoginServiceClient.FindMemInfoById(ctx, &login.UserMessage{MemId: taskInfo.AssignTo})
		if err != nil {
			zap.L().Error("project task TaskList LoginServiceClient.FindMemInfoById error", zap.Error(err))
			return nil, err
		}
		if memberMessage == nil {
			return nil, errs.GrpcError(model.DBError)
		}
		display.Executor = data.Executor{
			Name:   memberMessage.Name,
			Avatar: memberMessage.Avatar,
		}
	}
	var taskMessage = &task.TaskMessage{}
	copier.Copy(taskMessage, display)
	return taskMessage, nil
}

// TaskDone changes the completion state of a task and records the action in
// the project activity stream. Keeping this operation in the task service
// makes the HTTP gateway and all future clients share the same permission and
// tenant checks as the read/write task APIs.
func (t *TaskService) TaskDone(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskMessage, error) {
	if msg == nil || msg.MemberId <= 0 || (msg.Done != 0 && msg.Done != 1) {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	if taskCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskInfo, err := t.requireTaskMember(ctx, taskCode, msg.MemberId)
	if err != nil {
		return nil, err
	}

	done := int(msg.Done)
	if taskInfo.Done != done {
		if done == data.Done {
			taskInfo.DoneBy = msg.MemberId
			taskInfo.DoneTime = time.Now().UnixMilli()
			taskInfo.ExecuteStatus = data.Done
		} else {
			taskInfo.DoneBy = 0
			taskInfo.DoneTime = 0
			taskInfo.ExecuteStatus = data.Wait
		}
		taskInfo.Done = done
		if err := t.transaction.Action(func(conn database.DbConn) error {
			return t.taskRepo.SaveTask(ctx, conn, taskInfo)
		}); err != nil {
			zap.L().Error("project task TaskDone save task error", zap.Error(err))
			return nil, errs.GrpcError(model.DBError)
		}
		logType := "redo"
		if done == data.Done {
			logType = "done"
		}
		if err := createProjectLog(ctx, t.projectLogRepo, taskInfo.ProjectCode, taskInfo.Id, taskInfo.Name, msg.MemberId, taskInfo.AssignTo, logType, "task"); err != nil {
			zap.L().Warn("project task TaskDone create project log error", zap.Error(err))
		}
	}

	// Read the updated display through the normal path so the response contains
	// project/stage/executor information in exactly the same shape as readTask.
	return t.ReadTask(ctx, msg)
}

// TaskEdit updates the small set of task fields exposed by the web client.
// Keeping the field validation here makes the gateway a thin transport layer
// and prevents a caller from modifying arbitrary task columns.
func (t *TaskService) TaskEdit(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskMessage, error) {
	if msg == nil || msg.MemberId <= 0 || strings.TrimSpace(msg.TaskCode) == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskInfo, err := t.requireTaskMember(ctx, encrypts.DecryptNoErr(msg.TaskCode), msg.MemberId)
	if err != nil {
		return nil, err
	}
	field := strings.ToLower(strings.TrimSpace(msg.EditField))
	value := msg.EditValue
	switch field {
	case "name":
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > 255 {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
		taskInfo.Name = value
	case "description":
		taskInfo.Description = value
	case "pri":
		pri, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil || pri < 0 || pri > 2 {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
		taskInfo.Pri = pri
	case "status":
		status, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil || status < 0 || status > 4 {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
		taskInfo.Status = status
	case "begin_time", "end_time":
		parsed := int64(0)
		if strings.TrimSpace(value) != "" {
			parsed = tms.ParseTime(strings.TrimSpace(value))
			if parsed <= 0 {
				return nil, errs.GrpcError(model.InvalidParameter)
			}
		}
		if field == "begin_time" {
			taskInfo.BeginTime = parsed
		} else {
			taskInfo.EndTime = parsed
		}
	case "work_time":
		workTime, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil || workTime < 0 {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
		taskInfo.WorkTime = workTime
	case "like", "star", "private":
		flag, parseErr := strconv.Atoi(strings.TrimSpace(value))
		if parseErr != nil || (flag != 0 && flag != 1) {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
		switch field {
		case "like":
			taskInfo.Like = flag
		case "star":
			taskInfo.Star = flag
		case "private":
			taskInfo.Private = flag
		}
	default:
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if err := t.transaction.Action(func(conn database.DbConn) error {
		return t.taskRepo.SaveTask(ctx, conn, taskInfo)
	}); err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	return t.ReadTask(ctx, msg)
}

// TaskAssign changes the executor while maintaining the task-member rows
// used by the detail panel. Both the caller and the target must belong to the
// same project.
func (t *TaskService) TaskAssign(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskMessage, error) {
	if msg == nil || msg.MemberId <= 0 || strings.TrimSpace(msg.TaskCode) == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskInfo, err := t.requireTaskMember(ctx, encrypts.DecryptNoErr(msg.TaskCode), msg.MemberId)
	if err != nil {
		return nil, err
	}
	assignTo := int64(0)
	if strings.TrimSpace(msg.AssignTo) != "" {
		assignTo = encrypts.DecryptNoErr(msg.AssignTo)
		if assignTo <= 0 {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
	}
	if assignTo > 0 {
		members, _, findErr := t.projectRepo.FindProjectMemberByPid(ctx, taskInfo.ProjectCode)
		if findErr != nil {
			return nil, errs.GrpcError(model.DBError)
		}
		inProject := false
		for _, member := range members {
			if member != nil && member.MemberCode == assignTo {
				inProject = true
				break
			}
		}
		if !inProject {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
	}
	members, _, findErr := t.taskRepo.FindTaskMemberPage(ctx, taskInfo.Id, 1, 1000)
	if findErr != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if err := t.transaction.Action(func(conn database.DbConn) error {
		taskInfo.AssignTo = assignTo
		if err := t.taskRepo.SaveTask(ctx, conn, taskInfo); err != nil {
			return err
		}
		var selected *data.TaskMember
		for _, member := range members {
			if member == nil {
				continue
			}
			member.IsExecutor = model.NoExecutor
			if member.MemberCode == assignTo {
				selected = member
				member.IsExecutor = model.Executor
			}
			if err := t.taskRepo.SaveTaskMember(ctx, conn, member); err != nil {
				return err
			}
		}
		if assignTo > 0 && selected == nil {
			return t.taskRepo.SaveTaskMember(ctx, conn, &data.TaskMember{
				TaskCode:   taskInfo.Id,
				MemberCode: assignTo,
				JoinTime:   time.Now().UnixMilli(),
				IsExecutor: model.Executor,
			})
		}
		return nil
	}); err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	return t.ReadTask(ctx, msg)
}

// FileAction provides the file-page mutations that are safe to perform from
// the task service. File ownership is checked through the file's project
// membership before any metadata is changed.
func (t *TaskService) FileAction(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskSourceMessage, error) {
	if msg == nil || msg.MemberId <= 0 || strings.TrimSpace(msg.FileCode) == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	fileID := encrypts.DecryptNoErr(msg.FileCode)
	if fileID <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	files, err := t.fileRepo.FindByIds(ctx, []int64{fileID})
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if len(files) == 0 || files[0] == nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	file := files[0]
	if err := t.requireProjectMember(ctx, file.ProjectCode, msg.MemberId); err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(msg.FileAction)) {
	case "edit":
		title := strings.TrimSpace(msg.FileTitle)
		if title == "" || len([]rune(title)) > 255 || strings.ContainsAny(title, "/\\") {
			return nil, errs.GrpcError(model.InvalidParameter)
		}
		file.Title = title
	case "recycle":
		file.Deleted = model.Deleted
		file.DeletedTime = time.Now().UnixMilli()
	case "recovery":
		file.Deleted = model.NoDeleted
		file.DeletedTime = 0
	case "delete":
		if err := t.fileRepo.Delete(ctx, file.Id); err != nil {
			return nil, errs.GrpcError(model.DBError)
		}
		if err := t.sourceLinkRepo.DeleteBySourceCode(ctx, file.Id); err != nil {
			return nil, errs.GrpcError(model.DBError)
		}
		return &task.TaskSourceMessage{SourceDetail: &task.SourceDetail{PathName: file.PathName}}, nil
	default:
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if err := t.fileRepo.Save(ctx, file); err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	return &task.TaskSourceMessage{}, nil
}

func (t *TaskService) ListTaskMember(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskMemberList, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//查询 task member表 根据memberCode去查询用户信息
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	if taskCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if msg.Page <= 0 {
		msg.Page = 1
	}
	if msg.PageSize <= 0 {
		msg.PageSize = 10
	}
	if _, err := t.requireTaskMember(c, taskCode, msg.MemberId); err != nil {
		return nil, err
	}
	taskMemberPage, total, err := t.taskRepo.FindTaskMemberPage(c, taskCode, msg.Page, msg.PageSize)
	if err != nil {
		zap.L().Error("project task TaskList taskRepo.FindTaskMemberPage error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	var mids []int64
	for _, v := range taskMemberPage {
		if v == nil || v.MemberCode <= 0 {
			continue
		}
		mids = append(mids, v.MemberCode)
	}
	messageList, err := rpc.LoginServiceClient.FindMemInfoByIds(ctx, &login.UserMessage{MIds: mids})
	if err != nil {
		zap.L().Error("project task ListTaskMember LoginServiceClient.FindMemInfoByIds error", zap.Error(err))
		return nil, err
	}
	if messageList == nil {
		return nil, errs.GrpcError(model.DBError)
	}
	mMap := make(map[int64]*login.MemberMessage, len(messageList.List))
	for _, v := range messageList.List {
		if v == nil {
			continue
		}
		mMap[v.Id] = v
	}
	var taskMemeberMemssages []*task.TaskMemberMessage
	for _, v := range taskMemberPage {
		if v == nil {
			continue
		}
		tm := &task.TaskMemberMessage{}
		tm.Code = encrypts.EncryptNoErr(v.MemberCode)
		tm.Id = v.Id
		message := mMap[v.MemberCode]
		if message != nil {
			tm.Name = message.Name
			tm.Avatar = message.Avatar
		}
		tm.IsExecutor = int32(v.IsExecutor)
		tm.IsOwner = int32(v.IsOwner)
		taskMemeberMemssages = append(taskMemeberMemssages, tm)
	}
	return &task.TaskMemberList{List: taskMemeberMemssages, Total: total}, nil
}
func (t *TaskService) TaskLog(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskLogList, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	if taskCode <= 0 || (msg.All != 0 && msg.All != 1) || (msg.Comment != 0 && msg.Comment != 1) {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	all := msg.All
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if msg.Page <= 0 {
		msg.Page = 1
	}
	if msg.PageSize <= 0 {
		msg.PageSize = 10
	}
	if _, err := t.requireTaskMember(c, taskCode, msg.MemberId); err != nil {
		return nil, err
	}
	var list []*data.ProjectLog
	var total int64
	var err error
	if all == 1 {
		//显示全部
		list, total, err = t.projectLogRepo.FindLogByTaskCode(c, taskCode, int(msg.Comment))
	}
	if all == 0 {
		//分页
		list, total, err = t.projectLogRepo.FindLogByTaskCodePage(c, taskCode, int(msg.Comment), int(msg.Page), int(msg.PageSize))
	}
	if err != nil {
		zap.L().Error("project task TaskLog projectLogRepo.FindLogByTaskCodePage error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if total == 0 {
		return &task.TaskLogList{}, nil
	}
	var displayList []*data.ProjectLogDisplay
	var mIdList []int64
	for _, v := range list {
		if v == nil {
			continue
		}
		mIdList = append(mIdList, v.MemberCode)
	}
	messageList, err := rpc.LoginServiceClient.FindMemInfoByIds(c, &login.UserMessage{MIds: mIdList})
	if err != nil {
		zap.L().Error("project task TaskLog LoginServiceClient.FindMemInfoByIds error", zap.Error(err))
		return nil, err
	}
	if messageList == nil {
		return nil, errs.GrpcError(model.DBError)
	}
	mMap := make(map[int64]*login.MemberMessage)
	for _, v := range messageList.List {
		if v == nil {
			continue
		}
		mMap[v.Id] = v
	}
	for _, v := range list {
		if v == nil {
			continue
		}
		display := v.ToDisplay()
		message := mMap[v.MemberCode]
		if message != nil {
			display.Member = data.Member{
				Name:   message.Name,
				Id:     message.Id,
				Avatar: message.Avatar,
				Code:   message.Code,
			}
		}
		displayList = append(displayList, display)
	}
	var l []*task.TaskLog
	copier.Copy(&l, displayList)
	return &task.TaskLogList{List: l, Total: total}, nil
}

func (t *TaskService) TaskWorkTimeList(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskWorkTimeResponse, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	if taskCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if _, err := t.requireTaskMember(ctx, taskCode, msg.MemberId); err != nil {
		return nil, err
	}
	list, err := t.taskWorkTimeDomain.TaskWorkTimeList(ctx, taskCode)
	if err != nil {
		return nil, errs.GrpcError(err)
	}
	var l []*task.TaskWorkTime
	copier.Copy(&l, list)
	return &task.TaskWorkTimeResponse{List: l, Total: int64(len(l))}, nil
}

func (t *TaskService) SaveTaskWorkTime(ctx context.Context, msg *task.TaskReqMessage) (*task.SaveTaskWorkTimeResponse, error) {
	if msg == nil || msg.TaskCode == "" || msg.Num <= 0 {
		return nil, errs.GrpcError(model.WorkTimeInvalid)
	}
	tmt := &data.TaskWorkTime{}
	tmt.CreateTime = time.Now().UnixMilli()
	tmt.BeginTime = msg.BeginTime
	tmt.Num = int(msg.Num)
	tmt.Content = msg.Content
	tmt.TaskCode = encrypts.DecryptNoErr(msg.TaskCode)
	if tmt.TaskCode <= 0 || msg.MemberId <= 0 || tmt.BeginTime <= 0 {
		return nil, errs.GrpcError(model.WorkTimeInvalid)
	}
	if _, err := t.requireTaskMember(ctx, tmt.TaskCode, msg.MemberId); err != nil {
		return nil, err
	}
	tmt.MemberCode = msg.MemberId
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err := t.taskWorkTimeRepo.Save(c, tmt)
	if err != nil {
		zap.L().Error("project task SaveTaskWorkTime taskWorkTimeRepo.Save error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	return &task.SaveTaskWorkTimeResponse{}, nil
}

func (t *TaskService) SaveTaskFile(ctx context.Context, msg *task.TaskFileReqMessage) (*task.TaskFileResponse, error) {
	if msg == nil || msg.TaskCode == "" || msg.ProjectCode == "" || msg.PathName == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	projectCode := encrypts.DecryptNoErr(msg.ProjectCode)
	organizationCode := encrypts.DecryptNoErr(msg.OrganizationCode)
	if taskCode <= 0 || projectCode <= 0 || organizationCode <= 0 || msg.MemberId <= 0 || msg.Size < 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskInfo, err := t.requireTaskMember(ctx, taskCode, msg.MemberId)
	if err != nil {
		return nil, err
	}
	projectInfo, err := t.projectRepo.FindProjectById(ctx, projectCode)
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if projectInfo == nil || taskInfo.ProjectCode != projectCode || projectInfo.OrganizationCode != organizationCode {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//存file表
	f := &data.File{
		PathName:         msg.PathName,
		Title:            msg.FileName,
		Extension:        msg.Extension,
		Size:             int(msg.Size),
		ObjectType:       "",
		OrganizationCode: organizationCode,
		TaskCode:         encrypts.DecryptNoErr(msg.TaskCode),
		ProjectCode:      projectCode,
		CreateBy:         msg.MemberId,
		CreateTime:       time.Now().UnixMilli(),
		Downloads:        0,
		Extra:            "",
		Deleted:          model.NoDeleted,
		FileType:         msg.FileType,
		FileUrl:          msg.FileUrl,
		DeletedTime:      0,
	}
	err = t.fileRepo.Save(ctx, f)
	if err != nil {
		zap.L().Error("project task SaveTaskFile fileRepo.Save error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	//存入source_link
	sl := &data.SourceLink{
		SourceType:       "file",
		SourceCode:       f.Id,
		LinkType:         "task",
		LinkCode:         taskCode,
		OrganizationCode: organizationCode,
		CreateBy:         msg.MemberId,
		CreateTime:       time.Now().UnixMilli(),
		Sort:             0,
	}
	err = t.sourceLinkRepo.Save(ctx, sl)
	if err != nil {
		zap.L().Error("project task SaveTaskFile sourceLinkRepo.Save error", zap.Error(err))
		if cleanupErr := t.fileRepo.Delete(ctx, f.Id); cleanupErr != nil {
			zap.L().Error("project task SaveTaskFile file cleanup error", zap.Error(cleanupErr), zap.Int64("fileId", f.Id))
		}
		return nil, errs.GrpcError(model.DBError)
	}
	return &task.TaskFileResponse{}, nil
}

func (t *TaskService) TaskSources(ctx context.Context, msg *task.TaskReqMessage) (*task.TaskSourceResponse, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	if taskCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if _, err := t.requireTaskMember(ctx, taskCode, msg.MemberId); err != nil {
		return nil, err
	}
	sourceLinks, err := t.sourceLinkRepo.FindByTaskCode(ctx, taskCode)
	if err != nil {
		zap.L().Error("project task SaveTaskFile sourceLinkRepo.FindByTaskCode error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if len(sourceLinks) == 0 {
		return &task.TaskSourceResponse{}, nil
	}
	var fIdList []int64
	for _, v := range sourceLinks {
		if v == nil || v.SourceCode <= 0 {
			continue
		}
		fIdList = append(fIdList, v.SourceCode)
	}
	files, err := t.fileRepo.FindByIds(ctx, fIdList)
	if err != nil {
		zap.L().Error("project task SaveTaskFile fileRepo.FindByIds error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	fMap := make(map[int64]*data.File)
	for _, v := range files {
		if v == nil {
			continue
		}
		fMap[v.Id] = v
	}
	var list []*data.SourceLinkDisplay
	for _, v := range sourceLinks {
		if v == nil || fMap[v.SourceCode] == nil {
			continue
		}
		list = append(list, v.ToDisplay(fMap[v.SourceCode]))
	}
	var slMsg []*task.TaskSourceMessage
	copier.Copy(&slMsg, list)
	return &task.TaskSourceResponse{List: slMsg}, nil
}

func (t *TaskService) CreateComment(ctx context.Context, msg *task.TaskReqMessage) (*task.CreateCommentResponse, error) {
	if msg == nil || msg.TaskCode == "" || msg.CommentContent == "" || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskCode := encrypts.DecryptNoErr(msg.TaskCode)
	if taskCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	taskById, err := t.requireTaskMember(ctx, taskCode, msg.MemberId)
	if err != nil {
		zap.L().Error("project task CreateComment fileRepo.FindTaskById error", zap.Error(err))
		return nil, err
	}
	pl := &data.ProjectLog{
		MemberCode:   msg.MemberId,
		Content:      msg.CommentContent,
		Remark:       msg.CommentContent,
		Type:         "createComment",
		CreateTime:   time.Now().UnixMilli(),
		SourceCode:   taskCode,
		ActionType:   "task",
		ToMemberCode: 0,
		IsComment:    model.Comment,
		ProjectCode:  taskById.ProjectCode,
		Icon:         "plus",
		IsRobot:      0,
	}
	if err := t.projectLogRepo.SaveProjectLog(ctx, pl); err != nil {
		zap.L().Error("project task CreateComment SaveProjectLog error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	return &task.CreateCommentResponse{}, nil
}
