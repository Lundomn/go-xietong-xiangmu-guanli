package project_service_v1

import (
	"context"
	"github.com/jinzhu/copier"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"test.com/project-common/encrypts"
	"test.com/project-common/errs"
	"test.com/project-common/tms"
	"test.com/project-grpc/project"
	"test.com/project-grpc/user/login"
	"test.com/project-project/internal/dao"
	"test.com/project-project/internal/data"
	"test.com/project-project/internal/data/menu"
	"test.com/project-project/internal/database"
	"test.com/project-project/internal/database/tran"
	"test.com/project-project/internal/repo"
	"test.com/project-project/internal/rpc"
	"test.com/project-project/pkg/model"
	"time"
)

type ProjectService struct {
	project.UnimplementedProjectServiceServer
	cache                  repo.Cache
	transaction            tran.Transaction
	menuRepo               repo.MenuRepo
	projectRepo            repo.ProjectRepo
	projectTemplateRepo    repo.ProjectTemplateRepo
	taskStagesTemplateRepo repo.TaskStagesTemplateRepo
	taskStagesRepo         repo.TaskStagesRepo
	projectLogRepo         repo.ProjectLogRepo
	taskRepo               repo.TaskRepo
}

func New() *ProjectService {
	return &ProjectService{
		cache:                  dao.Rc,
		transaction:            dao.NewTransaction(),
		menuRepo:               dao.NewMenuDao(),
		projectRepo:            dao.NewProjectDao(),
		projectTemplateRepo:    dao.NewProjectTemplateDao(),
		taskStagesTemplateRepo: dao.NewTaskStagesTemplateDao(),
		taskStagesRepo:         dao.NewTaskStagesDao(),
		projectLogRepo:         dao.NewProjectLogDao(),
		taskRepo:               dao.NewTaskDao(),
	}
}

func (p *ProjectService) Index(ctx context.Context, _ *project.IndexMessage) (*project.IndexResponse, error) {
	pms, err := p.menuRepo.FindMenus(ctx)
	if err != nil {
		zap.L().Error("Index db FindMenus error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	childs := menu.CovertChild(pms)
	var mms []*project.MenuMessage
	copier.Copy(&mms, childs)
	return &project.IndexResponse{Menus: mms}, nil
}

func (p *ProjectService) FindProjectByMemId(ctx context.Context, msg *project.ProjectRpcMessage) (*project.MyProjectResponse, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	memberId := msg.MemberId
	page := msg.Page
	pageSize := msg.PageSize
	var pms []*data.ProjectAndMember
	var total int64
	var err error
	if msg.SelectBy == "" || msg.SelectBy == "my" {
		pms, total, err = p.projectRepo.FindProjectByMemId(ctx, memberId, "and deleted=0 ", page, pageSize)
	}
	if msg.SelectBy == "archive" {
		pms, total, err = p.projectRepo.FindProjectByMemId(ctx, memberId, "and archive=1 and deleted=0 ", page, pageSize)
	}
	if msg.SelectBy == "deleted" {
		pms, total, err = p.projectRepo.FindProjectByMemId(ctx, memberId, "and deleted=1 ", page, pageSize)
	}
	if msg.SelectBy != "" && msg.SelectBy != "my" && msg.SelectBy != "archive" && msg.SelectBy != "deleted" && msg.SelectBy != "collect" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if msg.SelectBy == "collect" {
		pms, total, err = p.projectRepo.FindCollectProjectByMemId(ctx, memberId, page, pageSize)
		for _, v := range pms {
			if v != nil {
				v.Collected = model.Collected
			}
		}
	} else {
		collectPms, _, err := p.projectRepo.FindCollectProjectByMemId(ctx, memberId, page, pageSize)
		if err != nil {
			zap.L().Error("project FindProjectByMemId::FindCollectProjectByMemId error", zap.Error(err))
			return nil, errs.GrpcError(model.DBError)
		}
		var cMap = make(map[int64]*data.ProjectAndMember)
		for _, v := range collectPms {
			if v == nil {
				continue
			}
			cMap[v.Id] = v
		}
		for _, v := range pms {
			if v == nil {
				continue
			}
			if cMap[v.ProjectCode] != nil {
				v.Collected = model.Collected
			}
		}
	}
	if err != nil {
		zap.L().Error("project FindProjectByMemId error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if pms == nil {
		return &project.MyProjectResponse{Pm: []*project.ProjectMessage{}, Total: total}, nil
	}

	var pmm []*project.ProjectMessage
	copier.Copy(&pmm, pms)
	for _, v := range pmm {
		if v == nil {
			continue
		}
		v.Code, _ = encrypts.EncryptInt64(v.ProjectCode, model.AESKey)
		pam := data.ToMap(pms)[v.Id]
		if pam == nil {
			continue
		}
		v.AccessControlType = pam.GetAccessControlType()
		v.OrganizationCode, _ = encrypts.EncryptInt64(pam.OrganizationCode, model.AESKey)
		v.JoinTime = tms.FormatByMill(pam.JoinTime)
		v.OwnerName = msg.MemberName
		v.Order = int32(pam.Sort)
		v.CreateTime = tms.FormatByMill(pam.CreateTime)
	}
	return &project.MyProjectResponse{Pm: pmm, Total: total}, nil
}

func (ps *ProjectService) FindProjectTemplate(ctx context.Context, msg *project.ProjectRpcMessage) (*project.ProjectTemplateResponse, error) {
	if msg == nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if msg.ViewType != 1 && msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//1.根据viewType去查询项目模板表 得到list
	organizationCodeStr, decryptErr := encrypts.Decrypt(msg.OrganizationCode, model.AESKey)
	if decryptErr != nil && msg.ViewType != 1 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	organizationCode, parseErr := strconv.ParseInt(organizationCodeStr, 10, 64)
	if (parseErr != nil || organizationCode <= 0) && msg.ViewType != 1 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	page := msg.Page
	pageSize := msg.PageSize
	var pts []data.ProjectTemplate
	var total int64
	var err error
	if msg.ViewType == -1 {
		pts, total, err = ps.projectTemplateRepo.FindProjectTemplateAll(ctx, organizationCode, page, pageSize)
	}
	if msg.ViewType == 0 {
		pts, total, err = ps.projectTemplateRepo.FindProjectTemplateCustom(ctx, msg.MemberId, organizationCode, page, pageSize)
	}
	if msg.ViewType == 1 {
		pts, total, err = ps.projectTemplateRepo.FindProjectTemplateSystem(ctx, page, pageSize)
	}
	if msg.ViewType != -1 && msg.ViewType != 0 && msg.ViewType != 1 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	if err != nil {
		zap.L().Error("project FindProjectTemplate FindProjectTemplateSystem error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	//2.模型转换，拿到模板id列表 去 任务步骤模板表 去进行查询
	tsts, err := ps.taskStagesTemplateRepo.FindInProTemIds(ctx, data.ToProjectTemplateIds(pts))
	if err != nil {
		zap.L().Error("project FindProjectTemplate FindInProTemIds error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	var ptas []*data.ProjectTemplateAll
	for _, v := range pts {
		//写代码 该谁做的事情一定要交出去
		ptas = append(ptas, v.Convert(data.CovertProjectMap(tsts)[v.Id]))
	}
	//3.组装数据
	var pmMsgs []*project.ProjectTemplateMessage
	copier.Copy(&pmMsgs, ptas)
	return &project.ProjectTemplateResponse{Ptm: pmMsgs, Total: total}, nil
}

func (ps *ProjectService) SaveProject(ctxs context.Context, msg *project.ProjectRpcMessage) (*project.SaveProjectMessage, error) {
	if msg == nil || msg.MemberId <= 0 || strings.TrimSpace(msg.Name) == "" || msg.OrganizationCode == "" || msg.TemplateCode == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	organizationCodeStr, err := encrypts.Decrypt(msg.OrganizationCode, model.AESKey)
	if err != nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	organizationCode, err := strconv.ParseInt(organizationCodeStr, 10, 64)
	if err != nil || organizationCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	templateCodeStr, err := encrypts.Decrypt(msg.TemplateCode, model.AESKey)
	if err != nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	templateCode, err := strconv.ParseInt(templateCodeStr, 10, 64)
	if err != nil || templateCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//获取模板信息
	ctx, cancel := context.WithTimeout(ctxs, 2*time.Second)
	defer cancel()
	template, err := ps.projectTemplateRepo.FindProjectTemplateById(ctx, int(templateCode))
	if err != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if template == nil || (template.IsSystem != 1 && template.OrganizationCode != organizationCode) {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	orgs, orgErr := rpc.LoginServiceClient.MyOrgList(ctx, &login.UserMessage{MemId: msg.MemberId})
	if orgErr != nil {
		return nil, orgErr
	}
	organizationBelongsToMember := false
	if orgs != nil {
		for _, org := range orgs.OrganizationList {
			if org != nil && org.Code == msg.OrganizationCode {
				organizationBelongsToMember = true
				break
			}
		}
	}
	if !organizationBelongsToMember {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	stageTemplateList, err := ps.taskStagesTemplateRepo.FindByProjectTemplateId(ctx, int(templateCode))
	if err != nil {
		zap.L().Error("project SaveProject taskStagesTemplateRepo.FindByProjectTemplateId error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if len(stageTemplateList) == 0 {
		return nil, errs.GrpcError(model.TaskStagesNotNull)
	}
	//1. 保存项目表
	pr := &data.Project{
		Name:              msg.Name,
		Description:       msg.Description,
		TemplateCode:      int(templateCode),
		CreateTime:        time.Now().UnixMilli(),
		Cover:             "https://img2.baidu.com/it/u=792555388,2449797505&fm=253&fmt=auto&app=138&f=JPEG?w=667&h=500",
		Deleted:           model.NoDeleted,
		Archive:           model.NoArchive,
		OrganizationCode:  organizationCode,
		AccessControlType: model.Open,
		TaskBoardTheme:    model.Simple,
	}
	err = ps.transaction.Action(func(conn database.DbConn) error {
		err := ps.projectRepo.SaveProject(conn, ctx, pr)
		if err != nil {
			zap.L().Error("project SaveProject SaveProject error", zap.Error(err))
			return errs.GrpcError(model.DBError)
		}
		pm := &data.ProjectMember{
			ProjectCode: pr.Id,
			MemberCode:  msg.MemberId,
			JoinTime:    time.Now().UnixMilli(),
			IsOwner:     msg.MemberId,
			Authorize:   "",
		}
		//2. 保存项目和成员的关联表
		err = ps.projectRepo.SaveProjectMember(conn, ctx, pm)
		if err != nil {
			zap.L().Error("project SaveProject SaveProjectMember error", zap.Error(err))
			return errs.GrpcError(model.DBError)
		}
		//3. 生成任务的步骤
		for index, v := range stageTemplateList {
			taskStage := &data.TaskStages{
				ProjectCode: pr.Id,
				Name:        v.Name,
				Sort:        index + 1,
				Description: "",
				CreateTime:  time.Now().UnixMilli(),
				Deleted:     model.NoDeleted,
			}
			err := ps.taskStagesRepo.SaveTaskStages(ctx, conn, taskStage)
			if err != nil {
				zap.L().Error("project SaveProject taskStagesRepo.SaveTaskStages error", zap.Error(err))
				return errs.GrpcError(model.DBError)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	code, _ := encrypts.EncryptInt64(pr.Id, model.AESKey)
	rsp := &project.SaveProjectMessage{
		Id:               pr.Id,
		Code:             code,
		Name:             pr.Name,
		Description:      pr.Description,
		Cover:            pr.Cover,
		CreateTime:       tms.FormatByMill(pr.CreateTime),
		TaskBoardTheme:   pr.TaskBoardTheme,
		OrganizationCode: msg.OrganizationCode,
	}
	return rsp, nil
}

// 1. 查项目表
// 2. 项目和成员的关联表 查到项目的拥有者 去member表查名字
// 3. 查收藏表 判断收藏状态
func (ps *ProjectService) FindProjectDetail(ctx context.Context, msg *project.ProjectRpcMessage) (*project.ProjectDetailMessage, error) {
	if msg == nil || msg.ProjectCode == "" || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCodeStr, err := encrypts.Decrypt(msg.ProjectCode, model.AESKey)
	if err != nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCode, err := strconv.ParseInt(projectCodeStr, 10, 64)
	if err != nil || projectCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	memberId := msg.MemberId
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	projectAndMember, err := ps.projectRepo.FindProjectByPIdAndMemId(c, projectCode, memberId)
	if err != nil {
		zap.L().Error("project FindProjectDetail FindProjectByPIdAndMemId error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if projectAndMember == nil || projectAndMember.Id <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	ownerId := projectAndMember.IsOwner
	member, err := rpc.LoginServiceClient.FindMemInfoById(c, &login.UserMessage{MemId: ownerId})
	if err != nil {
		zap.L().Error("project rpc FindProjectDetail FindMemInfoById error", zap.Error(err))
		return nil, err
	}
	if member == nil {
		return nil, errs.GrpcError(model.DBError)
	}
	//去user模块去找了
	//TODO 优化 收藏的时候 可以放入redis
	isCollect, err := ps.projectRepo.FindCollectByPidAndMemId(c, projectCode, memberId)
	if err != nil {
		zap.L().Error("project FindProjectDetail FindCollectByPidAndMemId error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if isCollect {
		projectAndMember.Collected = model.Collected
	}
	var detailMsg = &project.ProjectDetailMessage{}
	copier.Copy(detailMsg, projectAndMember)
	detailMsg.OwnerAvatar = member.Avatar
	detailMsg.OwnerName = member.Name
	detailMsg.Code, _ = encrypts.EncryptInt64(projectAndMember.Id, model.AESKey)
	detailMsg.AccessControlType = projectAndMember.GetAccessControlType()
	detailMsg.OrganizationCode, _ = encrypts.EncryptInt64(projectAndMember.OrganizationCode, model.AESKey)
	detailMsg.Order = int32(projectAndMember.Sort)
	detailMsg.CreateTime = tms.FormatByMill(projectAndMember.CreateTime)
	return detailMsg, nil
}
func (ps *ProjectService) UpdateDeletedProject(ctx context.Context, msg *project.ProjectRpcMessage) (*project.DeletedProjectResponse, error) {
	if msg == nil || msg.ProjectCode == "" || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCodeStr, err := encrypts.Decrypt(msg.ProjectCode, model.AESKey)
	if err != nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCode, err := strconv.ParseInt(projectCodeStr, 10, 64)
	if err != nil || projectCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	projectMember, memberErr := ps.projectRepo.FindProjectByPIdAndMemId(c, projectCode, msg.MemberId)
	if memberErr != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if projectMember == nil || projectMember.Id <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	err = ps.projectRepo.UpdateDeletedProject(c, projectCode, msg.Deleted)
	if err != nil {
		zap.L().Error("project RecycleProject DeleteProject error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	return &project.DeletedProjectResponse{}, nil
}
func (ps *ProjectService) UpdateProject(ctx context.Context, msg *project.UpdateProjectMessage) (*project.UpdateProjectResponse, error) {
	if msg == nil || msg.ProjectCode == "" || msg.MemberId <= 0 || strings.TrimSpace(msg.Name) == "" {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCodeStr, err := encrypts.Decrypt(msg.ProjectCode, model.AESKey)
	if err != nil {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	projectCode, err := strconv.ParseInt(projectCodeStr, 10, 64)
	if err != nil || projectCode <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	projectMember, memberErr := ps.projectRepo.FindProjectByPIdAndMemId(c, projectCode, msg.MemberId)
	if memberErr != nil {
		return nil, errs.GrpcError(model.DBError)
	}
	if projectMember == nil || projectMember.Id <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	proj := &data.Project{
		Id:                 projectCode,
		Name:               msg.Name,
		Description:        msg.Description,
		Cover:              msg.Cover,
		TaskBoardTheme:     msg.TaskBoardTheme,
		Prefix:             msg.Prefix,
		Private:            int(msg.Private),
		OpenPrefix:         int(msg.OpenPrefix),
		OpenBeginTime:      int(msg.OpenBeginTime),
		OpenTaskPrivate:    int(msg.OpenTaskPrivate),
		Schedule:           msg.Schedule,
		AutoUpdateSchedule: int(msg.AutoUpdateSchedule),
	}
	err = ps.projectRepo.UpdateProject(c, proj)
	if err != nil {
		zap.L().Error("project UpdateProject::UpdateProject error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	return &project.UpdateProjectResponse{}, nil
}

func (ps *ProjectService) GetLogBySelfProject(ctx context.Context, msg *project.ProjectRpcMessage) (*project.ProjectLogResponse, error) {
	if msg == nil || msg.MemberId <= 0 {
		return nil, errs.GrpcError(model.InvalidParameter)
	}
	//根据用户id查询当前的用户的日志表

	page, pageSize := msg.Page, msg.PageSize
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	projectLogs, total, err := ps.projectLogRepo.FindLogByMemberCode(ctx, msg.MemberId, page, pageSize)
	if err != nil {
		zap.L().Error("project ProjectService::GetLogBySelfProject projectLogRepo.FindLogByMemberCode error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	if len(projectLogs) == 0 {
		return &project.ProjectLogResponse{List: []*project.ProjectLogMessage{}, Total: total}, nil
	}
	//查询项目信息
	pIdList := make([]int64, 0, len(projectLogs))
	mIdList := make([]int64, 0, len(projectLogs))
	taskIdList := make([]int64, 0, len(projectLogs))
	for _, v := range projectLogs {
		if v == nil {
			continue
		}
		pIdList = append(pIdList, v.ProjectCode)
		mIdList = append(mIdList, v.MemberCode)
		taskIdList = append(taskIdList, v.SourceCode)
	}
	projects, err := ps.projectRepo.FindProjectByIds(ctx, pIdList)
	if err != nil {
		zap.L().Error("project ProjectService::GetLogBySelfProject projectLogRepo.FindProjectByIds error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	pMap := make(map[int64]*data.Project)
	for _, v := range projects {
		if v == nil {
			continue
		}
		pMap[v.Id] = v
	}
	messageList, err := rpc.LoginServiceClient.FindMemInfoByIds(ctx, &login.UserMessage{MIds: mIdList})
	if err != nil {
		zap.L().Error("project ProjectService::GetLogBySelfProject FindMemInfoByIds error", zap.Error(err))
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
	tasks, err := ps.taskRepo.FindTaskByIds(ctx, taskIdList)
	if err != nil {
		zap.L().Error("project ProjectService::GetLogBySelfProject projectLogRepo.FindTaskByIds error", zap.Error(err))
		return nil, errs.GrpcError(model.DBError)
	}
	tMap := make(map[int64]*data.Task)
	for _, v := range tasks {
		if v == nil {
			continue
		}
		tMap[v.Id] = v
	}
	var list []*data.IndexProjectLogDisplay
	for _, v := range projectLogs {
		if v == nil {
			continue
		}
		display := v.ToIndexDisplay()
		if project := pMap[v.ProjectCode]; project != nil {
			display.ProjectName = project.Name
		}
		if member := mMap[v.MemberCode]; member != nil {
			display.MemberAvatar = member.Avatar
			display.MemberName = member.Name
		}
		if task := tMap[v.SourceCode]; task != nil {
			display.TaskName = task.Name
		}
		list = append(list, display)
	}
	var msgList []*project.ProjectLogMessage
	copier.Copy(&msgList, list)
	return &project.ProjectLogResponse{List: msgList, Total: total}, nil
}
