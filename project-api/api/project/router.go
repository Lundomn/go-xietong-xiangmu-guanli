package project

import (
	"github.com/gin-gonic/gin"
	"log"
	"test.com/project-api/api/midd"
	"test.com/project-api/router"
)

type RouterProject struct {
}

func init() {
	log.Println("init project router")
	ru := &RouterProject{}
	router.Register(ru)
}

func (*RouterProject) Route(r *gin.Engine) {
	//初始化grpc的客户端连接
	InitRpcProjectClient()
	h := New()
	group := r.Group("/project")
	group.Use(midd.TokenVerify())
	group.POST("/index", h.index)
	group.POST("/project/selfList", h.myProjectList)
	group.POST("/project", h.myProjectList)
	group.POST("/project_template", h.projectTemplate)
	group.POST("/project/save", h.projectSave)
	group.POST("/project/read", h.readProject)
	group.POST("/project/recycle", h.recycleProject)
	group.POST("/project/recovery", h.recoveryProject)
	group.POST("/project_collect/collect", h.collectProject)
	group.POST("/project/edit", h.editProject)
	group.POST("/project/getLogBySelfProject", h.getLogBySelfProject)
	group.POST("/project/_projectStats", h.projectStats)
	group.POST("/project/_getProjectReport", h.projectReport)
	group.POST("/report/weekly", h.weeklyReport)
	t := NewTask()
	// Attachments are served through an authenticated task-aware handler instead
	// of exposing the upload directory as a public static filesystem.
	group.GET("/upload/*filepath", midd.TokenVerify(), t.downloadFile)
	group.POST("/task_stages", t.taskStages)
	group.POST("/project_member/index", t.memberProjectList)
	group.POST("/task_stages/tasks", t.taskList)
	group.POST("/task/save", t.saveTask)
	group.POST("/task/sort", t.taskSort)
	group.POST("/task/selfList", t.myTaskList)
	group.POST("/task/read", t.readTask)
	group.POST("/task/taskDone", t.taskDone)
	group.POST("/task/edit", t.editTask)
	group.POST("/task/like", t.editTask)
	group.POST("/task/star", t.editTask)
	group.POST("/task/setPrivate", t.editTask)
	group.POST("/task/assignTask", t.assignTask)
	group.POST("/task_member", t.listTaskMember)
	group.POST("/task/taskLog", t.taskLog)
	group.POST("/task/_taskWorkTimeList", t.taskWorkTimeList)
	group.POST("/task/saveTaskWorkTime", t.saveTaskWorkTime)
	group.POST("/task/dateTotalForProject", t.dateTotalForProject)
	group.POST("/file/uploadFiles", t.uploadFiles)
	group.POST("/file", t.projectFiles)
	group.POST("/file/edit", t.editFile)
	group.POST("/file/recycle", t.recycleFile)
	group.POST("/file/recovery", t.recoveryFile)
	group.POST("/file/delete", t.deleteFile)
	group.POST("/task/taskSources", t.taskSources)
	group.POST("/task/createComment", t.createComment)

	a := NewAccount()
	group.POST("/account", a.account)
	d := NewDepartment()
	group.POST("/department", d.department)
	group.POST("/department/save", d.save)
	group.POST("/department/read", d.read)
	auth := NewAuth()
	group.POST("/auth", auth.authList)
}
