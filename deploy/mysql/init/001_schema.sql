-- The official MySQL image supplies MYSQL_DATABASE as the init connection's
-- default database. Do not hard-code a database name here so deployments can
-- use the value configured in .env.
-- It also runs init scripts with a latin1 client by default, so force UTF-8
-- for Chinese seed data and future schema changes.
SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS ms_member (
    id BIGINT NOT NULL AUTO_INCREMENT,
    account VARCHAR(128) NOT NULL DEFAULT '',
    password VARCHAR(128) NOT NULL DEFAULT '',
    name VARCHAR(128) NOT NULL DEFAULT '',
    mobile VARCHAR(32) NOT NULL DEFAULT '',
    realname VARCHAR(128) NOT NULL DEFAULT '',
    create_time BIGINT NOT NULL DEFAULT 0,
    status INT NOT NULL DEFAULT 1,
    last_login_time BIGINT NOT NULL DEFAULT 0,
    sex INT NOT NULL DEFAULT 0,
    avatar VARCHAR(1024) NOT NULL DEFAULT '',
    idcard VARCHAR(64) NOT NULL DEFAULT '',
    province INT NOT NULL DEFAULT 0,
    city INT NOT NULL DEFAULT 0,
    area INT NOT NULL DEFAULT 0,
    address VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL,
    email VARCHAR(255) NOT NULL DEFAULT '',
    dingtalk_openid VARCHAR(255) NOT NULL DEFAULT '',
    dingtalk_unionid VARCHAR(255) NOT NULL DEFAULT '',
    dingtalk_userid VARCHAR(255) NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    KEY idx_member_account (account),
    KEY idx_member_mobile (mobile),
    KEY idx_member_email (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_organization (
    id BIGINT NOT NULL AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL DEFAULT '',
    avatar VARCHAR(1024) NOT NULL DEFAULT '',
    description TEXT NOT NULL,
    member_id BIGINT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    personal INT NOT NULL DEFAULT 0,
    address VARCHAR(255) NOT NULL DEFAULT '',
    province INT NOT NULL DEFAULT 0,
    city INT NOT NULL DEFAULT 0,
    area INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_organization_member (member_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_project_template (
    id INT NOT NULL AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL,
    sort INT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    organization_code BIGINT NOT NULL DEFAULT 0,
    cover VARCHAR(1024) NOT NULL DEFAULT '',
    member_code BIGINT NOT NULL DEFAULT 0,
    is_system INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_project_template_system (is_system),
    KEY idx_project_template_org (organization_code, member_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_task_stages_template (
    id INT NOT NULL AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL DEFAULT '',
    project_template_code INT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    sort INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_task_stage_template_project (project_template_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_project (
    id BIGINT NOT NULL AUTO_INCREMENT,
    cover VARCHAR(1024) NOT NULL DEFAULT '',
    name VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL,
    access_control_type INT NOT NULL DEFAULT 0,
    white_list TEXT NOT NULL,
    sort INT NOT NULL DEFAULT 0,
    deleted INT NOT NULL DEFAULT 0,
    template_code INT NOT NULL DEFAULT 0,
    schedule DOUBLE NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    organization_code BIGINT NOT NULL DEFAULT 0,
    deleted_time VARCHAR(64) NOT NULL DEFAULT '',
    private INT NOT NULL DEFAULT 0,
    prefix VARCHAR(32) NOT NULL DEFAULT '',
    open_prefix INT NOT NULL DEFAULT 0,
    archive INT NOT NULL DEFAULT 0,
    archive_time BIGINT NOT NULL DEFAULT 0,
    open_begin_time INT NOT NULL DEFAULT 0,
    open_task_private INT NOT NULL DEFAULT 0,
    task_board_theme VARCHAR(64) NOT NULL DEFAULT '',
    begin_time BIGINT NOT NULL DEFAULT 0,
    end_time BIGINT NOT NULL DEFAULT 0,
    auto_update_schedule INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_project_org_deleted (organization_code, deleted),
    KEY idx_project_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_project_member (
    id BIGINT NOT NULL AUTO_INCREMENT,
    project_code BIGINT NOT NULL DEFAULT 0,
    member_code BIGINT NOT NULL DEFAULT 0,
    join_time BIGINT NOT NULL DEFAULT 0,
    is_owner BIGINT NOT NULL DEFAULT 0,
    authorize TEXT NOT NULL,
    PRIMARY KEY (id),
    KEY idx_project_member_project (project_code),
    KEY idx_project_member_member (member_code),
    UNIQUE KEY uk_project_member (project_code, member_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_project_collection (
    id BIGINT NOT NULL AUTO_INCREMENT,
    project_code BIGINT NOT NULL DEFAULT 0,
    member_code BIGINT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    UNIQUE KEY uk_project_collection (project_code, member_code),
    KEY idx_project_collection_member (member_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_task_stages (
    id INT NOT NULL AUTO_INCREMENT,
    name VARCHAR(255) NOT NULL DEFAULT '',
    project_code BIGINT NOT NULL DEFAULT 0,
    sort INT NOT NULL DEFAULT 0,
    description TEXT NOT NULL,
    create_time BIGINT NOT NULL DEFAULT 0,
    deleted INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_task_stages_project (project_code, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_task (
    id BIGINT NOT NULL AUTO_INCREMENT,
    project_code BIGINT NOT NULL DEFAULT 0,
    name VARCHAR(255) NOT NULL DEFAULT '',
    pri INT NOT NULL DEFAULT 0,
    execute_status INT NOT NULL DEFAULT 0,
    description TEXT NOT NULL,
    create_by BIGINT NOT NULL DEFAULT 0,
    done_by BIGINT NOT NULL DEFAULT 0,
    done_time BIGINT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    assign_to BIGINT NOT NULL DEFAULT 0,
    deleted INT NOT NULL DEFAULT 0,
    stage_code INT NOT NULL DEFAULT 0,
    task_tag VARCHAR(255) NOT NULL DEFAULT '',
    done INT NOT NULL DEFAULT 0,
    begin_time BIGINT NOT NULL DEFAULT 0,
    end_time BIGINT NOT NULL DEFAULT 0,
    remind_time BIGINT NOT NULL DEFAULT 0,
    pcode BIGINT NOT NULL DEFAULT 0,
    sort INT NOT NULL DEFAULT 0,
    `like` INT NOT NULL DEFAULT 0,
    star INT NOT NULL DEFAULT 0,
    deleted_time BIGINT NOT NULL DEFAULT 0,
    private INT NOT NULL DEFAULT 0,
    id_num INT NOT NULL DEFAULT 0,
    path VARCHAR(1024) NOT NULL DEFAULT '',
    schedule INT NOT NULL DEFAULT 0,
    version_code BIGINT NOT NULL DEFAULT 0,
    features_code BIGINT NOT NULL DEFAULT 0,
    work_time INT NOT NULL DEFAULT 0,
    status INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_task_project_stage (project_code, stage_code, deleted),
    KEY idx_task_assign_done (assign_to, deleted, done),
    KEY idx_task_create_done (create_by, deleted, done)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_task_member (
    id BIGINT NOT NULL AUTO_INCREMENT,
    task_code BIGINT NOT NULL DEFAULT 0,
    is_executor INT NOT NULL DEFAULT 0,
    member_code BIGINT NOT NULL DEFAULT 0,
    join_time BIGINT NOT NULL DEFAULT 0,
    is_owner INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_task_member_task (task_code),
    KEY idx_task_member_member (member_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_project_log (
    id BIGINT NOT NULL AUTO_INCREMENT,
    member_code BIGINT NOT NULL DEFAULT 0,
    content TEXT NOT NULL,
    remark TEXT NOT NULL,
    type VARCHAR(64) NOT NULL DEFAULT '',
    create_time BIGINT NOT NULL DEFAULT 0,
    source_code BIGINT NOT NULL DEFAULT 0,
    action_type VARCHAR(64) NOT NULL DEFAULT '',
    to_member_code BIGINT NOT NULL DEFAULT 0,
    is_comment INT NOT NULL DEFAULT 0,
    project_code BIGINT NOT NULL DEFAULT 0,
    icon VARCHAR(255) NOT NULL DEFAULT '',
    is_robot INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_project_log_member (member_code, create_time),
    KEY idx_project_log_source (source_code, is_comment),
    KEY idx_project_log_project (project_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_file (
    id BIGINT NOT NULL AUTO_INCREMENT,
    path_name VARCHAR(1024) NOT NULL DEFAULT '',
    title VARCHAR(255) NOT NULL DEFAULT '',
    extension VARCHAR(32) NOT NULL DEFAULT '',
    `size` BIGINT NOT NULL DEFAULT 0,
    object_type VARCHAR(64) NOT NULL DEFAULT '',
    organization_code BIGINT NOT NULL DEFAULT 0,
    task_code BIGINT NOT NULL DEFAULT 0,
    project_code BIGINT NOT NULL DEFAULT 0,
    create_by BIGINT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    downloads INT NOT NULL DEFAULT 0,
    extra TEXT NOT NULL,
    deleted INT NOT NULL DEFAULT 0,
    file_url VARCHAR(2048) NOT NULL DEFAULT '',
    file_type VARCHAR(255) NOT NULL DEFAULT '',
    deleted_time BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_file_task (task_code),
    KEY idx_file_project (project_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_source_link (
    id BIGINT NOT NULL AUTO_INCREMENT,
    source_type VARCHAR(64) NOT NULL DEFAULT '',
    source_code BIGINT NOT NULL DEFAULT 0,
    link_type VARCHAR(64) NOT NULL DEFAULT '',
    link_code BIGINT NOT NULL DEFAULT 0,
    organization_code BIGINT NOT NULL DEFAULT 0,
    create_by BIGINT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    sort INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_source_link (link_type, link_code),
    KEY idx_source_code (source_type, source_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_task_work_time (
    id BIGINT NOT NULL AUTO_INCREMENT,
    task_code BIGINT NOT NULL DEFAULT 0,
    member_code BIGINT NOT NULL DEFAULT 0,
    create_time BIGINT NOT NULL DEFAULT 0,
    content TEXT NOT NULL,
    begin_time BIGINT NOT NULL DEFAULT 0,
    num INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_task_work_time_task (task_code),
    KEY idx_task_work_time_member (member_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_department (
    id BIGINT NOT NULL AUTO_INCREMENT,
    organization_code BIGINT NOT NULL DEFAULT 0,
    name VARCHAR(255) NOT NULL DEFAULT '',
    sort INT NOT NULL DEFAULT 0,
    pcode BIGINT NOT NULL DEFAULT 0,
    icon VARCHAR(255) NOT NULL DEFAULT '',
    create_time BIGINT NOT NULL DEFAULT 0,
    path VARCHAR(1024) NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    KEY idx_department_org_parent (organization_code, pcode),
    UNIQUE KEY uk_department_name (organization_code, pcode, name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_member_account (
    id BIGINT NOT NULL AUTO_INCREMENT,
    organization_code BIGINT NOT NULL DEFAULT 0,
    department_code BIGINT NOT NULL DEFAULT 0,
    member_code BIGINT NOT NULL DEFAULT 0,
    authorize TEXT NOT NULL,
    is_owner INT NOT NULL DEFAULT 0,
    name VARCHAR(128) NOT NULL DEFAULT '',
    mobile VARCHAR(32) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    create_time BIGINT NOT NULL DEFAULT 0,
    last_login_time BIGINT NOT NULL DEFAULT 0,
    status INT NOT NULL DEFAULT 1,
    description TEXT NOT NULL,
    avatar VARCHAR(1024) NOT NULL DEFAULT '',
    position VARCHAR(255) NOT NULL DEFAULT '',
    department VARCHAR(255) NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    KEY idx_member_account_org (organization_code),
    KEY idx_member_account_member (member_code),
    KEY idx_member_account_department (department_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_project_auth (
    id BIGINT NOT NULL AUTO_INCREMENT,
    organization_code BIGINT NOT NULL DEFAULT 0,
    title VARCHAR(255) NOT NULL DEFAULT '',
    create_at BIGINT NOT NULL DEFAULT 0,
    sort INT NOT NULL DEFAULT 0,
    status INT NOT NULL DEFAULT 1,
    `desc` TEXT NOT NULL,
    create_by BIGINT NOT NULL DEFAULT 0,
    is_default INT NOT NULL DEFAULT 0,
    type VARCHAR(64) NOT NULL DEFAULT '',
    PRIMARY KEY (id),
    KEY idx_project_auth_org (organization_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ms_project_menu (
    id BIGINT NOT NULL AUTO_INCREMENT,
    pid BIGINT NOT NULL DEFAULT 0,
    title VARCHAR(255) NOT NULL DEFAULT '',
    icon VARCHAR(255) NOT NULL DEFAULT '',
    url VARCHAR(1024) NOT NULL DEFAULT '',
    file_path VARCHAR(1024) NOT NULL DEFAULT '',
    params VARCHAR(1024) NOT NULL DEFAULT '',
    node VARCHAR(255) NOT NULL DEFAULT '',
    sort INT NOT NULL DEFAULT 0,
    status INT NOT NULL DEFAULT 1,
    create_by BIGINT NOT NULL DEFAULT 0,
    is_inner INT NOT NULL DEFAULT 0,
    `values` VARCHAR(255) NOT NULL DEFAULT '',
    show_slider INT NOT NULL DEFAULT 0,
    PRIMARY KEY (id),
    KEY idx_project_menu_pid_sort (pid, sort)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO ms_project_template
    (id, name, description, sort, create_time, organization_code, cover, member_code, is_system)
SELECT 1, '基础项目', '默认项目模板', 1, UNIX_TIMESTAMP(CURRENT_TIMESTAMP) * 1000, 0, '', 0, 1
WHERE NOT EXISTS (SELECT 1 FROM ms_project_template WHERE id = 1);

INSERT INTO ms_task_stages_template (name, project_template_code, create_time, sort)
SELECT '待办', 1, UNIX_TIMESTAMP(CURRENT_TIMESTAMP) * 1000, 3
WHERE NOT EXISTS (SELECT 1 FROM ms_task_stages_template WHERE project_template_code = 1 AND name = '待办');
INSERT INTO ms_task_stages_template (name, project_template_code, create_time, sort)
SELECT '进行中', 1, UNIX_TIMESTAMP(CURRENT_TIMESTAMP) * 1000, 2
WHERE NOT EXISTS (SELECT 1 FROM ms_task_stages_template WHERE project_template_code = 1 AND name = '进行中');
INSERT INTO ms_task_stages_template (name, project_template_code, create_time, sort)
SELECT '已完成', 1, UNIX_TIMESTAMP(CURRENT_TIMESTAMP) * 1000, 1
WHERE NOT EXISTS (SELECT 1 FROM ms_task_stages_template WHERE project_template_code = 1 AND name = '已完成');

INSERT INTO ms_project_menu
    (id, pid, title, icon, url, file_path, params, node, sort, status, create_by, is_inner, `values`, show_slider)
VALUES
    (1, 0, '工作台', 'home', '/home', 'home/index', '', 'home', 1, 1, 0, 0, '', 0),
    (2, 0, '项目管理', 'project', '/project', '', '', 'project', 2, 1, 0, 0, '', 1),
    (3, 2, '项目列表', 'list', '/project/list/my', 'project/list/index', '', 'project-list', 1, 1, 0, 0, '', 1),
    (4, 2, '我的任务', 'task', '/task', 'home/index', '', 'task-list', 2, 1, 0, 1, '', 1)
ON DUPLICATE KEY UPDATE
    pid = VALUES(pid),
    title = VALUES(title),
    icon = VALUES(icon),
    url = VALUES(url),
    file_path = VALUES(file_path),
    node = VALUES(node),
    sort = VALUES(sort),
    status = VALUES(status),
    is_inner = VALUES(is_inner),
    show_slider = VALUES(show_slider);
