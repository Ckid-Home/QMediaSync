package controllers

import (
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"Q115-STRM/internal/notificationmanager"
	"Q115-STRM/internal/synccron"
	"context"
	"strconv"
	"time"
)

// TaskType 任务类型枚举
type TaskType string

const (
	TaskTypeStrm   TaskType = "strm"
	TaskTypeScrape TaskType = "scrape"
)

// runTask 执行指定类型的任务并在完成后发送通知
// args: 可选参数，传入目录ID时只执行指定目录的任务
// taskType: 任务类型（strm或scrape）
// isFullSync: 是否执行全量同步（仅适用于strm任务）
func runTask(args []string, taskType TaskType, isFullSync bool) string {
	// 检查参数格式
	if len(args) > 0 && args[0] != "" {
		param := args[0]
		// 检查参数是否以#开头且长度大于1
		if !(len(param) > 1 && param[0] == '#') {
			return "❌ 参数格式错误，请使用 #数字 格式"
		}
		// 尝试将参数转换为uint
		numStr := param[1:]
		_, parseErr := strconv.ParseUint(numStr, 10, 32)
		if parseErr != nil {
			return "❌ 参数格式错误，请使用 #数字 格式"
		}
	}

	// 先返回开始执行的消息
	go func() {
		var taskIDs []uint
		var taskTypeSynccron synccron.SyncTaskType
		var title, content string

		// 设置任务类型和通知信息
		switch taskType {
		case TaskTypeStrm:
			taskTypeSynccron = synccron.SyncTaskTypeStrm
			if isFullSync {
				title = "✅ 全量STRM同步完成"
				content = "所有全量STRM同步任务已执行完毕"
			} else {
				title = "✅ 增量STRM同步完成"
				content = "所有增量STRM同步任务已执行完毕"
			}
		case TaskTypeScrape:
			taskTypeSynccron = synccron.SyncTaskTypeScrape
			title = "✅ 刮削任务完成"
			content = "所有刮削任务已执行完毕"
		default:
			return
		}

		// 检查是否传入了目录ID
		if len(args) > 0 && args[0] != "" {
			// 处理"#数字"格式的参数
			param := args[0]
			// 去掉#符号
			numStr := param[1:]
			// 尝试将参数转换为uint
			id, parseErr := strconv.ParseUint(numStr, 10, 32)
			if parseErr == nil {
				taskID := uint(id)

				// 根据任务类型处理
				switch taskType {
				case TaskTypeStrm:
					// 获取指定同步目录
					syncPath := models.GetSyncPathById(taskID)
					if syncPath != nil {
						// 如果是全量同步，设置标志
						if isFullSync {
							syncPath.SetIsFullSync(true)
						}
						// 同步指定目录
						synccron.AddNewSyncTask(taskID, taskTypeSynccron)
						taskIDs = []uint{taskID}
						// 设置通知内容
						if isFullSync {
							content = "目录：" + syncPath.RemotePath + "，全量STRM同步任务已执行完毕"
						} else {
							content = "目录：" + syncPath.RemotePath + "，增量STRM同步任务已执行完毕"
						}
					}
				case TaskTypeScrape:
					// 获取指定刮削目录
					scrapePath := models.GetScrapePathByID(taskID)
					if scrapePath != nil {
						// 执行刮削任务
						synccron.AddNewSyncTask(taskID, taskTypeSynccron)
						taskIDs = []uint{taskID}
						// 设置通知内容
						content = "目录：" + scrapePath.SourcePath + "，刮削任务已执行完毕"
					}
				}
			}
		}

		// 如果没有指定目录，执行所有目录
		if len(taskIDs) == 0 {
			switch taskType {
			case TaskTypeStrm:
				if isFullSync {
					// 获取所有同步目录
					allSyncPaths, _ := models.GetSyncPathList(1, 10000000, true)
					for _, syncPath := range allSyncPaths {
						// 设置为全量同步
						syncPath.SetIsFullSync(true)
						// 同步目录
						synccron.AddNewSyncTask(syncPath.ID, taskTypeSynccron)
						taskIDs = append(taskIDs, syncPath.ID)
					}
					// 设置通知内容
					content = "目录：全部，全量STRM同步任务已执行完毕"
				} else {
					// 增量同步所有目录
					synccron.StartSyncCron()
					// 获取所有同步目录
					allSyncPaths, _ := models.GetSyncPathList(1, 10000000, true)
					for _, syncPath := range allSyncPaths {
						taskIDs = append(taskIDs, syncPath.ID)
					}
					// 设置通知内容
					content = "目录：全部，增量STRM同步任务已执行完毕"
				}
			case TaskTypeScrape:
				// 获取所有刮削目录
				allScrapePaths := models.GetScrapePathes()
				for _, scrapePath := range allScrapePaths {
					// 执行刮削任务
					synccron.AddNewSyncTask(scrapePath.ID, taskTypeSynccron)
					taskIDs = append(taskIDs, scrapePath.ID)
				}
				// 设置通知内容
				content = "目录：全部，刮削任务已执行完毕"
			}
		}

		// 检查是否有任务
		if len(taskIDs) == 0 {
			return
		}

		// 等待所有任务执行完成
		time.Sleep(2 * time.Second) // 等待任务队列初始化

		// 监控任务的状态
		waitForTasksCompletion(taskIDs, taskTypeSynccron)

		// 所有任务执行完成，发送通知
		ctx := context.Background()
		notif := &models.Notification{
			Type:      models.SystemAlert,
			Title:     title,
			Content:   content,
			Timestamp: time.Now(),
			Priority:  models.NormalPriority,
		}
		if notificationmanager.GlobalEnhancedNotificationManager != nil {
			notificationmanager.GlobalEnhancedNotificationManager.SendNotification(ctx, notif)
		}
	}()

	// 返回开始执行的消息
	switch taskType {
	case TaskTypeStrm:
		if isFullSync {
			return "🔄 开始执行全量STRM同步"
		}
		return "🔄 开始执行增量STRM同步"
	case TaskTypeScrape:
		return "🔄 开始执行刮削任务"
	default:
		return "🔄 开始执行任务"
	}
}

// SyncStrmInc 执行增量STRM同步并在完成后发送通知
// args: 可选参数，传入同步目录ID时只同步指定目录
func SyncStrmInc(args []string) string {
	return runTask(args, TaskTypeStrm, false)
}

// SyncStrnFull 执行全量STRM同步并在完成后发送通知
// args: 可选参数，传入同步目录ID时只同步指定目录
func SyncStrnFull(args []string) string {
	return runTask(args, TaskTypeStrm, true)
}

// Scrape 执行刮削任务并在完成后发送通知
// args: 可选参数，传入刮削目录ID时只执行指定目录的刮削
func Scrape(args []string) string {
	return runTask(args, TaskTypeScrape, false)
}

// parseTaskID 解析任务ID参数
func parseTaskID(param string) (uint, bool) {
	if len(param) > 1 && param[0] == '#' {
		numStr := param[1:]
		id, parseErr := strconv.ParseUint(numStr, 10, 32)
		if parseErr == nil {
			return uint(id), true
		}
	}
	return 0, false
}

// waitForTasksCompletion 等待指定任务完成
func waitForTasksCompletion(taskIDs []uint, taskType synccron.SyncTaskType) {
	if len(taskIDs) == 0 {
		return
	}
	allCompleted := false
	for !allCompleted {
		time.Sleep(5 * time.Second)
		allCompleted = true
		for _, taskID := range taskIDs {
			status := synccron.CheckNewTaskStatus(taskID, taskType)
			if status == synccron.TaskStatusWaiting || status == synccron.TaskStatusRunning {
				allCompleted = false
				break
			}
		}
	}
}

// runTaskSequence 执行任务序列
// taskTypes: 任务类型序列，如 []TaskType{TaskTypeScrape, TaskTypeStrm}
// args: 参数列表，格式为 #数字 #数字
// title: 完成通知的标题
func runTaskSequence(taskTypes []TaskType, args []string, title string) string {
	// 检查参数格式
	if len(args) > 0 {
		for _, arg := range args {
			if arg != "" && !(len(arg) > 1 && arg[0] == '#') {
				return "❌ 参数格式错误，请使用 #数字 #数字 格式"
			}
		}
	}

	// 先返回开始执行的消息
	go func() {
		// 解析参数
		taskIDs := make([]uint, len(taskTypes))
		handleAllPaths := make([]bool, len(taskTypes))
		for i := range handleAllPaths {
			handleAllPaths[i] = true
		}

		for i := 0; i < len(taskTypes) && i < len(args); i++ {
			if args[i] != "" {
				if id, ok := parseTaskID(args[i]); ok {
					taskIDs[i] = id
					handleAllPaths[i] = (id == 0)
				}
			}
		}

		// 记录任务执行情况
		var taskResults []string

		// 执行任务序列
		for i, taskType := range taskTypes {
			var currentTaskIDs []uint
			var taskTypeSynccron synccron.SyncTaskType
			// 记录是否有新的刮削文件
			var hasNewScrapeFiles bool

			switch taskType {
			case TaskTypeStrm:
				taskTypeSynccron = synccron.SyncTaskTypeStrm
			case TaskTypeScrape:
				taskTypeSynccron = synccron.SyncTaskTypeScrape
			}

			if handleAllPaths[i] {
				// 执行所有目录的任务
				if taskType == TaskTypeStrm {
					synccron.StartSyncCron()
					allSyncPaths, _ := models.GetSyncPathList(1, 10000000, true)
					for _, syncPath := range allSyncPaths {
						currentTaskIDs = append(currentTaskIDs, syncPath.ID)
					}
					taskResults = append(taskResults, "目录：全部，增量STRM同步已完成")
				} else {
					allScrapePaths := models.GetScrapePathes()
					for _, scrapePath := range allScrapePaths {
						// 刮削开始前检查是否有新文件
						newScrapeFilesCount := models.GetScannedScrapeMediaFilesTotal(scrapePath.ID, scrapePath.MediaType)
						if newScrapeFilesCount > 0 {
							hasNewScrapeFiles = true
						}
						// 执行刮削任务
						synccron.AddNewSyncTask(scrapePath.ID, taskTypeSynccron)
						currentTaskIDs = append(currentTaskIDs, scrapePath.ID)
					}
					taskResults = append(taskResults, "目录：全部，刮削已完成")
				}
			} else {
				// 执行指定目录的任务
				if taskType == TaskTypeStrm {
					syncPath := models.GetSyncPathById(taskIDs[i])
					if syncPath != nil {
						synccron.AddNewSyncTask(taskIDs[i], taskTypeSynccron)
						currentTaskIDs = []uint{taskIDs[i]}
						taskResults = append(taskResults, "目录："+syncPath.RemotePath+"，增量STRM同步已完成")
					}
				} else {
					scrapePath := models.GetScrapePathByID(taskIDs[i])
					if scrapePath != nil {
						// 刮削开始前检查是否有新文件
						newScrapeFilesCount := models.GetScannedScrapeMediaFilesTotal(scrapePath.ID, scrapePath.MediaType)
						if newScrapeFilesCount > 0 {
							hasNewScrapeFiles = true
						}
						// 执行刮削任务
						synccron.AddNewSyncTask(taskIDs[i], taskTypeSynccron)
						currentTaskIDs = []uint{taskIDs[i]}
						taskResults = append(taskResults, "目录："+scrapePath.SourcePath+"，刮削已完成")
					}
				}
			}

			// 等待任务开始执行
			time.Sleep(5 * time.Second)

			// 监控任务完成
			waitForTasksCompletion(currentTaskIDs, taskTypeSynccron)

			// 只在第一个任务后等待上传下载任务完成
			if i == 0 {
				time.Sleep(15 * time.Second)
			}

			// 刮削任务完成后，如果是SyncThenScrape序列（先同步后刮削）且有新文件，触发Emby媒体库刷新
			if taskType == TaskTypeScrape && len(taskTypes) > 1 && taskTypes[0] == TaskTypeStrm && hasNewScrapeFiles {
				var refreshIDs []uint
				// 对于SyncThenScrape序列，使用同步任务的ID（第一个任务）而不是刮削任务的ID
				if !handleAllPaths[0] && taskIDs[0] > 0 {
					// 使用同步任务的ID
					syncPath := models.GetSyncPathById(taskIDs[0])
					if syncPath != nil {
						refreshIDs = append(refreshIDs, taskIDs[0])
						helpers.AppLogger.Infof("添加同步目录到Emby刷新列表: %s (ID: %d)", syncPath.RemotePath, taskIDs[0])
					}
				} else if handleAllPaths[0] {
					// 如果是全部同步，使用所有同步目录的ID
					allSyncPaths, _ := models.GetSyncPathList(1, 10000000, true)
					for _, syncPath := range allSyncPaths {
						refreshIDs = append(refreshIDs, syncPath.ID)
						helpers.AppLogger.Infof("添加同步目录到Emby刷新列表: %s (ID: %d)", syncPath.RemotePath, syncPath.ID)
					}
				}

				// 如果有需要刷新的目录，等待30秒后执行刷新
				if len(refreshIDs) > 0 {
					// 等待30秒，确保文件操作完成
					go func(ids []uint) {
						time.Sleep(30 * time.Second)
						// 对需要刷新的目录触发Emby媒体库刷新
						for _, taskID := range ids {
							models.RefreshEmbyLibraryBySyncPathId(taskID)
						}
					}(refreshIDs)
				}
			}
		}

		// 构建通知内容
		content := ""
		for _, result := range taskResults {
			content += result + "\n"
		}
		if content == "" {
			content = "所有任务已全部执行完毕"
		}

		// 发送完成通知
		ctx := context.Background()
		notif := &models.Notification{
			Type:      models.SystemAlert,
			Title:     title,
			Content:   content,
			Timestamp: time.Now(),
			Priority:  models.NormalPriority,
		}
		if notificationmanager.GlobalEnhancedNotificationManager != nil {
			notificationmanager.GlobalEnhancedNotificationManager.SendNotification(ctx, notif)
		}
	}()

	return "🔄 开始执行任务序列"
}

// ScrapeThenSync 先执行刮削任务，完成后再执行同步任务
// args: 参数格式为 #数字 #数字，分别代表刮削目录ID和同步目录ID
// 如果参数为0，则执行所有目录的操作
func ScrapeThenSync(args []string) string {
	return runTaskSequence([]TaskType{TaskTypeScrape, TaskTypeStrm}, args, "✅ 刮削后同步完成")
}

// SyncThenScrape 先执行同步任务，完成后再执行刮削任务
// args: 参数格式为 #数字 #数字，分别代表同步目录ID和刮削目录ID
// 如果参数为0，则执行所有目录的操作
func SyncThenScrape(args []string) string {
	return runTaskSequence([]TaskType{TaskTypeStrm, TaskTypeScrape}, args, "✅ 同步后刮削完成")
}

func StartListenTelegramBot() {
	mgr := notificationmanager.GlobalEnhancedNotificationManager

	myCommands := map[string]func([]string) string{
		"strm_inc":    SyncStrmInc,
		"strm_sync":   SyncStrnFull,
		"scrape":      Scrape,
		"scrape_sync": ScrapeThenSync,
		"sync_scrape": SyncThenScrape,
	}

	mgr.RegisterTelegramCommands(myCommands)
	mgr.StartAll()
}
