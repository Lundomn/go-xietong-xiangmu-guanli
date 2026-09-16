/**
 * Home
 */
export default [
    {
        // 工作台首页。菜单由后端动态返回，但页面本身保持静态可访问。
        name: 'home',
        path: '/home/:organizationCode?',
        component: resolve => require(['@/views/home/index'], resolve),
        meta: {model: 1, info: {id: 1, title: '工作台', fullUrl: 'home', show_slider: false, is_inner: true}},
    },
    {
        name: 'projectList',
        path: '/project/list/:type?',
        component: resolve => require(['@/views/project/list/index'], resolve),
        meta: {model: 2, info: {id: 3, title: '项目列表', fullUrl: 'project/list/my', show_slider: true}},
    },
    {
        //任务看板
        name: 'task',
        path: '/project/space/task/:code',
        component: resolve => require(['@/views/project/space/task'], resolve),
        meta: {model: 2, info: {id: 4, title: '我的任务', fullUrl: 'task', show_slider: false, is_inner: true}},
        children: [
            {
                //任务详情
                name: 'taskdetail',
                path: 'detail/:taskCode',
                component: resolve => require(['@/views/project/space/taskdetail'], resolve),
                meta: {model: 'Project', info: {show_slider: false}},
            },
        ]
    },
    {
        name: 'projectOverview',
        path: '/project/space/overview/:code',
        component: resolve => require(['@/views/project/space/overview'], resolve),
        meta: {model: 2, info: {id: 3, title: '项目概览', fullUrl: 'project/space/overview', show_slider: false, is_inner: true}},
    },
    {
        name: 'projectFiles',
        path: '/project/space/files/:code',
        component: resolve => require(['@/views/project/space/files'], resolve),
        meta: {model: 2, info: {id: 3, title: '项目文件', fullUrl: 'project/space/files', show_slider: false, is_inner: true}},
    },
    {
        //邀请链接
        name: 'inviteFromLink',
        path: '/invite_from_link/:code',
        component: resolve => require(['@/views/common/inviteFromLink'], resolve),
        meta: {model: 'Common', info: {show_slider: false}},
    },
    {
        name: 'calendar',
        path: '/calendar',
        component: resolve => require(['@/views/common/calendar'], resolve),
        meta: {model: 'Common', info: {show_slider: false}},
    },
];
