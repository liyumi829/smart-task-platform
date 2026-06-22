-- 创建数据库（如果已经有库可以跳过）
CREATE DATABASE IF NOT EXISTS `smart_task_platform`
DEFAULT CHARACTER SET utf8mb4
DEFAULT COLLATE utf8mb4_unicode_ci;

USE `smart_task_platform`;

-- ============================================================
-- 用户表
-- ============================================================
CREATE TABLE `users` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '用户 ID',
    `username` VARCHAR(50) NOT NULL COMMENT '用户名',
    `email` VARCHAR(100) NOT NULL COMMENT '邮箱',
    `password_hash` VARCHAR(255) NOT NULL COMMENT '密码哈希',
    `nickname` VARCHAR(50) DEFAULT NULL COMMENT '昵称',
    `avatar` VARCHAR(255) DEFAULT NULL COMMENT '头像 URL',
    `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '用户状态',
    `last_login_at` DATETIME DEFAULT NULL COMMENT '最后登录时间',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME DEFAULT NULL COMMENT '软删除时间',

    PRIMARY KEY (`id`),

    -- 用户名唯一索引
    -- 对应：
    -- GetByAccount
    -- ExistsByUsername
    -- SearchUsers 的 username 前缀匹配
    UNIQUE KEY `uk_users_username` (
        `username`
    ),

    -- 邮箱唯一索引
    -- 对应：
    -- GetByAccount
    -- ExistsByEmail
    UNIQUE KEY `uk_users_email` (
        `email`
    ),

    -- 用户搜索列表索引
    -- 对应：
    -- WHERE status = 'active'
    --   AND deleted_at IS NULL
    --   AND username LIKE 'xxx%'
    -- ORDER BY username ASC, id DESC
    KEY `idx_users_search_active` (
        `status`,
        `deleted_at`,
        `username` ASC,
        `id` DESC
    )

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='用户表';

-- ============================================================
-- 项目表
-- ============================================================
CREATE TABLE `projects` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '项目 ID',
    `name` VARCHAR(100) NOT NULL COMMENT '项目名称',
    `description` TEXT NULL COMMENT '项目描述',
    `owner_id` BIGINT UNSIGNED NOT NULL COMMENT '创建者 / 拥有者 ID',
    `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT '项目状态',
    `start_date` DATE NULL COMMENT '项目开始日期',
    `end_date` DATE NULL COMMENT '项目结束日期',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME NULL DEFAULT NULL COMMENT '软删除时间',

    PRIMARY KEY (`id`),

    -- 外键字段索引，必须保留
    KEY `idx_projects_owner_id` (
        `owner_id`
    ),

    -- 项目默认列表索引
    -- 对应：
    -- WHERE deleted_at IS NULL
    -- ORDER BY created_at DESC, name ASC, id DESC
    KEY `idx_projects_default_list` (
        `deleted_at`,
        `created_at` DESC,
        `name` ASC,
        `id` DESC
    ),

    -- 如果 status 筛选很常用，可以加
    KEY `idx_projects_status_default_list` (
        `status`,
        `deleted_at`,
        `created_at` DESC,
        `name` ASC,
        `id` DESC
    ),


    CONSTRAINT `fk_projects_owner_id`
        FOREIGN KEY (`owner_id`) REFERENCES `users` (`id`)
        ON UPDATE CASCADE
        ON DELETE RESTRICT
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='项目表';

-- ============================================================
-- 项目成员表
-- ============================================================
CREATE TABLE `project_members` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '记录 ID',
    `project_id` BIGINT UNSIGNED NOT NULL COMMENT '项目 ID',
    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '用户 ID',
    `role` VARCHAR(20) NOT NULL DEFAULT 'member' COMMENT '项目角色',
    `invited_by` BIGINT UNSIGNED NULL COMMENT '邀请人 ID',
    `joined_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '加入时间',
    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME NULL COMMENT '删除时间（软删除）',

    PRIMARY KEY (`id`),

    -- 保证同一个项目下同一个用户只能有一条成员记录
    UNIQUE KEY `uk_project_user` (`project_id`, `user_id`),

    KEY `idx_project_members_user_id` (`user_id`),
    KEY `idx_project_members_role` (`role`),
    KEY `idx_project_members_invited_by` (`invited_by`),
    KEY `idx_project_members_deleted_at` (`deleted_at`),

    -- 权限校验高频索引
    KEY `idx_project_members_project_user_deleted` (`project_id`, `user_id`, `deleted_at`),
    KEY `idx_project_members_user_project_deleted` (`user_id`, `project_id`, `deleted_at`),


    CONSTRAINT `fk_project_members_project_id`
        FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`)
        ON UPDATE CASCADE
        ON DELETE CASCADE,

    CONSTRAINT `fk_project_members_user_id`
        FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
        ON UPDATE CASCADE
        ON DELETE CASCADE,

    CONSTRAINT `fk_project_members_invited_by`
        FOREIGN KEY (`invited_by`) REFERENCES `users` (`id`)
        ON UPDATE CASCADE
        ON DELETE SET NULL
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='项目成员表';

-- ============================================================
-- 任务表
-- ============================================================
CREATE TABLE `tasks` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '任务ID',

    `project_id` BIGINT UNSIGNED NOT NULL COMMENT '所属项目ID',
    `title` VARCHAR(200) NOT NULL COMMENT '任务标题',
    `description` TEXT NULL COMMENT '任务描述',

    `status` VARCHAR(20) NOT NULL DEFAULT 'todo' COMMENT '任务状态：todo/in_progress/done/cancelled',
    `priority` VARCHAR(20) NOT NULL DEFAULT 'medium' COMMENT '任务优先级：low/medium/high/urgent',

    `creator_id` BIGINT UNSIGNED NOT NULL COMMENT '创建人ID',
    `assignee_id` BIGINT UNSIGNED NULL DEFAULT NULL COMMENT '负责人ID',

    `due_date` DATETIME NULL DEFAULT NULL COMMENT '截止时间',
    `start_time` DATETIME NULL DEFAULT NULL COMMENT '开始时间',
    `completed_at` DATETIME NULL DEFAULT NULL COMMENT '完成时间',

    `ai_summary` TEXT NULL COMMENT 'AI任务摘要',
    `sort_order` INT NOT NULL DEFAULT 0 COMMENT '排序值',

    `priority_order` TINYINT GENERATED ALWAYS AS (
        CASE `priority`
            WHEN 'urgent' THEN 1
            WHEN 'high' THEN 2
            WHEN 'medium' THEN 3
            WHEN 'low' THEN 4
            ELSE 3
        END
    ) STORED COMMENT '优先级排序值：1 urgent，2 high，3 medium，4 low',

    `status_order` TINYINT GENERATED ALWAYS AS (
        CASE `status`
            WHEN 'todo' THEN 1
            WHEN 'in_progress' THEN 2
            WHEN 'done' THEN 3
            WHEN 'cancelled' THEN 4
            ELSE 1
        END
    ) STORED COMMENT '状态排序值：1 todo，2 in_progress，3 done，4 cancelled',

    `due_date_null_order` TINYINT GENERATED ALWAYS AS (
        CASE
            WHEN `due_date` IS NULL THEN 1
            ELSE 0
        END
    ) STORED COMMENT '截止时间 NULL 排序值：0 非空，1 NULL',

    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME NULL DEFAULT NULL COMMENT '软删除时间',

    PRIMARY KEY (`id`),

    -- 创建人外键索引，当前没有其它 creator_id 前缀组合索引，所以保留
    KEY `idx_tasks_creator_id` (
        `creator_id`
    ),

    -- 搜索窄索引
    `idx_tasks_project_deleted_count` (
        `project_id`,
        `deleted_at`
    ),

    -- 项目任务默认列表索引
    -- 对应：
    -- WHERE project_id = ?
    --   AND deleted_at IS NULL
    -- ORDER BY sort_order ASC,
    --          priority_order ASC,
    --          status_order ASC,
    --          title ASC,
    --          due_date_null_order ASC,
    --          due_date ASC,
    --          created_at DESC,
    --          id DESC
    KEY `idx_tasks_project_default_sort` (
        `project_id` ASC,
        `deleted_at` ASC,
        `sort_order` ASC,
        `priority_order` ASC,
        `status_order` ASC,
        `title` ASC,
        `due_date_null_order` ASC,
        `due_date` ASC,
        `created_at` DESC,
        `id` DESC
    ),

    -- 我的任务默认列表索引
    -- 对应：
    -- WHERE assignee_id = ?
    --   AND deleted_at IS NULL
    -- ORDER BY sort_order ASC,
    --          priority_order ASC,
    --          status_order ASC,
    --          title ASC,
    --          due_date_null_order ASC,
    --          due_date ASC,
    --          created_at DESC,
    --          id DESC
    --
    -- 同时满足 assignee_id 外键索引要求
    KEY `idx_tasks_assignee_default_sort` (
        `assignee_id` ASC,
        `deleted_at` ASC,
        `sort_order` ASC,
        `priority_order` ASC,
        `status_order` ASC,
        `title` ASC,
        `due_date_null_order` ASC,
        `due_date` ASC,
        `created_at` DESC,
        `id` DESC
    ),

    -- 项目内按状态筛选任务列表
    -- 对应：
    -- WHERE project_id = ?
    --   AND status = ?
    --   AND deleted_at IS NULL
    -- ORDER BY 默认排序字段
    KEY `idx_tasks_project_status_sort` (
        `project_id` ASC,
        `status` ASC,
        `deleted_at` ASC,
        `sort_order` ASC,
        `priority_order` ASC,
        `status_order` ASC,
        `title` ASC,
        `due_date_null_order` ASC,
        `due_date` ASC,
        `created_at` DESC,
        `id` DESC
    ),

    -- 项目内按优先级筛选任务列表
    -- 对应：
    -- WHERE project_id = ?
    --   AND priority = ?
    --   AND deleted_at IS NULL
    -- ORDER BY 默认排序字段
    KEY `idx_tasks_project_priority_sort` (
        `project_id` ASC,
        `priority` ASC,
        `deleted_at` ASC,
        `sort_order` ASC,
        `priority_order` ASC,
        `status_order` ASC,
        `title` ASC,
        `due_date_null_order` ASC,
        `due_date` ASC,
        `created_at` DESC,
        `id` DESC
    ),

    -- 项目内按负责人筛选任务列表
    -- 对应：
    -- WHERE project_id = ?
    --   AND assignee_id = ?
    --   AND deleted_at IS NULL
    -- ORDER BY 默认排序字段
    --
    -- 同时适配：
    -- ClearTaskAssigneeByProjectIDAndAssigneeIDWithTx
    KEY `idx_tasks_project_assignee_sort` (
        `project_id` ASC,
        `assignee_id` ASC,
        `deleted_at` ASC,
        `sort_order` ASC,
        `priority_order` ASC,
        `status_order` ASC,
        `title` ASC,
        `due_date_null_order` ASC,
        `due_date` ASC,
        `created_at` DESC,
        `id` DESC
    ),

    -- 项目内标题前缀搜索索引
    -- 对应：
    -- WHERE project_id = ?
    --   AND deleted_at IS NULL
    --   AND title LIKE 'xxx%'
    KEY `idx_tasks_project_title` (
        `project_id` ASC,
        `deleted_at` ASC,
        `title` ASC,
        `id` DESC
    ),

    -- 外键约束
    CONSTRAINT `fk_tasks_project_id`
        FOREIGN KEY (`project_id`) REFERENCES `projects` (`id`)
        ON UPDATE CASCADE
        ON DELETE RESTRICT,

    CONSTRAINT `fk_tasks_creator_id`
        FOREIGN KEY (`creator_id`) REFERENCES `users` (`id`)
        ON UPDATE CASCADE
        ON DELETE RESTRICT,

    CONSTRAINT `fk_tasks_assignee_id`
        FOREIGN KEY (`assignee_id`) REFERENCES `users` (`id`)
        ON UPDATE CASCADE
        ON DELETE SET NULL

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='任务表';

-- ============================================================
-- 任务评论表
-- ============================================================
CREATE TABLE `task_comments` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '评论 ID',

    `task_id` BIGINT UNSIGNED NOT NULL COMMENT '任务 ID',
    `author_id` BIGINT UNSIGNED NOT NULL COMMENT '评论人 ID',

    `parent_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '父评论 ID，支持回复',
    `reply_to_user_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '被回复用户 ID',

    `content` TEXT NOT NULL COMMENT '评论内容',

    `created_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    `updated_at` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    `deleted_at` DATETIME DEFAULT NULL COMMENT '软删除时间',

    PRIMARY KEY (`id`),
    -- 外键索引
    KEY `idx_task_comments_author_id` (
        `author_id`
    ),
    KEY `idx_task_comments_parent_id` (
        `parent_id`
    ),
    KEY `idx_task_comments_reply_to_user_id` (
        `reply_to_user_id`
    ),
    
    KEY `idx_task_comments_task_list` (
        `task_id`,
        `deleted_at`,
        `created_at` DESC,
        `id` DESC
    ),


    CONSTRAINT `fk_task_comments_task_id`
        FOREIGN KEY (`task_id`) REFERENCES `tasks` (`id`)
        ON UPDATE CASCADE
        ON DELETE CASCADE,

    CONSTRAINT `fk_task_comments_user_id`
        FOREIGN KEY (`author_id`) REFERENCES `users` (`id`)
        ON UPDATE CASCADE
        ON DELETE RESTRICT,

    CONSTRAINT `fk_task_comments_parent_id`
        FOREIGN KEY (`parent_id`) REFERENCES `task_comments` (`id`)
        ON UPDATE CASCADE
        ON DELETE SET NULL,

    CONSTRAINT `fk_task_comments_reply_to_user_id`
        FOREIGN KEY (`reply_to_user_id`) REFERENCES `users` (`id`)
        ON UPDATE CASCADE
        ON DELETE SET NULL
) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='任务评论表';

-- =========================================================
-- 任务活动表
-- =========================================================

CREATE TABLE IF NOT EXISTS `task_activities` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '任务动态ID',

    `task_id` BIGINT UNSIGNED NOT NULL COMMENT '任务ID',
    `project_id` BIGINT UNSIGNED NOT NULL COMMENT '项目ID',
    `operator_id` BIGINT UNSIGNED NOT NULL COMMENT '操作人ID',

    `action` VARCHAR(50) NOT NULL COMMENT '动作类型，例如 task_created/task_assigned/task_status_changed/comment_added',
    `content` VARCHAR(500) NOT NULL COMMENT '动态内容',

    `related_type` VARCHAR(50) DEFAULT NULL COMMENT '关联资源类型，例如 task/comment/project',
    `related_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '关联资源ID',

    `extra_json` JSON DEFAULT NULL COMMENT '扩展JSON，记录变更前后内容等信息',

    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',

    PRIMARY KEY (`id`),

    -- 核心索引：服务 ListByTaskID，支持 WHERE task_id = ? ORDER BY created_at DESC, id DESC
    KEY `idx_task_activities_task_created_id` (`task_id`, `created_at` DESC, `id` DESC)

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='任务动态表';

-- =========================================================
-- 通知表
-- =========================================================

CREATE TABLE IF NOT EXISTS `notifications` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '通知ID',

    `user_id` BIGINT UNSIGNED NOT NULL COMMENT '接收通知的用户ID',
    `sender_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '发送人/触发人ID，系统通知可为空',

    `type` VARCHAR(50) NOT NULL COMMENT '通知类型，例如 task_assigned/task_status_changed/comment_reply/system',
    `title` VARCHAR(200) NOT NULL COMMENT '通知标题',
    `content` VARCHAR(500) NOT NULL COMMENT '通知内容',

    `is_read` TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否已读：0未读，1已读',
    `read_at` DATETIME(3) DEFAULT NULL COMMENT '阅读时间',

    `related_type` VARCHAR(50) DEFAULT NULL COMMENT '关联资源类型，例如 task/comment/project/system',
    `related_id` BIGINT UNSIGNED DEFAULT NULL COMMENT '关联资源ID',

    `pushed_at` DATETIME(3) DEFAULT NULL COMMENT 'WebSocket 推送成功时间',

    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',
    `deleted_at` DATETIME(3) DEFAULT NULL COMMENT '软删除时间',

    PRIMARY KEY (`id`),

    -- 核心索引：服务 ListByUserID
    -- WHERE user_id = ? AND deleted_at IS NULL ORDER BY created_at DESC, id DESC
    KEY `idx_notifications_user_deleted_created_id` (`user_id`, `deleted_at`, `created_at` DESC, `id` DESC),

    -- 核心索引：服务 CountUnreadByUserID / MarkAllAsRead
    -- WHERE user_id = ? AND is_read = 0 AND deleted_at IS NULL
    KEY `idx_notifications_user_read_deleted_created_id` (`user_id`, `is_read`, `deleted_at`, `created_at` DESC, `id` DESC)

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='用户通知表';

-- =========================================================
-- 发件箱消息表
-- =========================================================

CREATE TABLE IF NOT EXISTS `outbox_messages` (
    `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT 'Outbox消息ID',

    `event_id` VARCHAR(64) NOT NULL COMMENT '事件唯一ID，用于幂等',
    `event_type` VARCHAR(100) NOT NULL COMMENT '事件类型，例如 notification.created/task.assigned',

    `exchange_name` VARCHAR(100) NOT NULL COMMENT '消息交换机名称',
    `routing_key` VARCHAR(100) NOT NULL COMMENT '消息路由键',

    `payload` JSON NOT NULL COMMENT '消息内容JSON',

    `status` VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT '状态：pending/processing/sent/failed',
    `retry_count` INT UNSIGNED NOT NULL DEFAULT 0 COMMENT '当前重试次数',
    `max_retry_count` INT UNSIGNED NOT NULL DEFAULT 5 COMMENT '最大重试次数',

    `next_retry_at` DATETIME(3) DEFAULT NULL COMMENT '下次允许重试时间',

    `locked_by` VARCHAR(100) DEFAULT NULL COMMENT '当前处理该消息的Worker标识',
    `locked_at` DATETIME(3) DEFAULT NULL COMMENT '锁定时间',

    `sent_at` DATETIME(3) DEFAULT NULL COMMENT '成功发送时间',
    `error_message` VARCHAR(1000) DEFAULT NULL COMMENT '最近一次错误信息',

    `created_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间',
    `updated_at` DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间',

    PRIMARY KEY (`id`),

    -- 幂等索引：防止同一个事件重复写入 Outbox
    UNIQUE KEY `uk_outbox_messages_event_id` (`event_id`),

    -- 核心索引：服务 ListPending
    -- WHERE status = 'pending'
    -- AND retry_count < max_retry_count
    -- AND (next_retry_at IS NULL OR next_retry_at <= ?)
    -- ORDER BY created_at ASC, id ASC
    KEY `idx_outbox_messages_status_retry_created_id` (`status`, `next_retry_at`, `created_at`, `id`),

    -- 核心索引：服务 ResetTimeoutProcessingMessages
    -- WHERE status = 'processing' AND locked_at < ?
    KEY `idx_outbox_messages_status_locked_at` (`status`, `locked_at`)

) ENGINE=InnoDB
  DEFAULT CHARSET=utf8mb4
  COLLATE=utf8mb4_unicode_ci
  COMMENT='Outbox消息表';