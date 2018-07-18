package main

import (
	"errors"
	"fmt"
	"jiacrontab/libs"
	"jiacrontab/libs/finder"
	"jiacrontab/libs/proto"
	"jiacrontab/model"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type CrontabTask struct {
}

func (t *CrontabTask) List(args struct{ Page, Pagesize int }, reply *[]model.CrontabTask) error {
	return model.DB().Offset(args.Page - 1).Limit(args.Pagesize).Find(reply).Error
}
func (t *CrontabTask) All(args string, reply *[]model.CrontabTask) error {
	return model.DB().Find(reply).Error
}

func (t *CrontabTask) Update(args model.CrontabTask, ok *bool) error {
	*ok = true
	if args.MailTo == "" {
		args.MailTo = globalConfig.mailTo
	}

	if args.ID == 0 {
		ret := model.DB().Create(&args)
		if ret.Error == nil {

			globalCrontab.add(&args)
		}

	} else {
		var crontabTask model.CrontabTask
		// ret := model.DB().Find(&crontabTask, "id=?", args.ID)

		err := globalCrontab.update(args.ID, func(t *model.CrontabTask) error {

			if t.NumberProcess > 0 {
				return errors.New("can not update when task is running")
			}
			t.Name = args.Name
			t.Command = args.Command
			t.Args = args.Args
			t.MailTo = args.MailTo
			t.Depends = args.Depends
			t.UnexpectedExitMail = args.UnexpectedExitMail
			t.PipeCommands = args.PipeCommands
			t.Sync = args.Sync
			t.Timeout = args.Timeout
			t.MaxConcurrent = args.MaxConcurrent
			if t.MaxConcurrent == 0 {
				t.MaxConcurrent = 1
			}

			t.MailTo = args.MailTo
			t.OpTimeout = args.OpTimeout
			t.C = args.C
			crontabTask = *t
			return nil

		})

		if err == nil {
			model.DB().Model(&model.CrontabTask{}).Where("id=? and number_process=0", crontabTask.ID).Update(map[string]interface{}{
				"name":                 crontabTask.Name,
				"command":              crontabTask.Command,
				"args":                 crontabTask.Args,
				"mail_to":              crontabTask.MailTo,
				"depends":              crontabTask.Depends,
				"upexpected_exit_mail": crontabTask.UnexpectedExitMail,
				"pipe_commands":        crontabTask.PipeCommands,
				"sync":                 crontabTask.Sync,
				"timeout":              crontabTask.Timeout,
				"max_concurrent":       crontabTask.MaxConcurrent,
				"op_timeout":           crontabTask.OpTimeout,
				"c":                    crontabTask.C,
			})

		} else {
			*ok = false
		}

	}

	return nil
}
func (t *CrontabTask) Get(args uint, reply *model.CrontabTask) error {
	return model.DB().Find(reply, "id=?", args).Error
}

func (t *CrontabTask) Start(args string, ok *bool) error {
	*ok = true
	ids := strings.Split(args, ",")
	for _, v := range ids {
		var crontabTask model.CrontabTask
		ret := model.DB().Find(&crontabTask, "id=?", libs.ParseInt(v))
		if ret.Error != nil {
			*ok = false
		} else {
			if crontabTask.TimerCounter == 0 {
				globalCrontab.add(&crontabTask)
			}
		}
	}

	return nil
}

func (t *CrontabTask) Stop(args string, ok *bool) error {
	*ok = true
	ids := strings.Split(args, ",")
	for _, v := range ids {
		var crontabTask model.CrontabTask
		fmt.Printf("id", libs.ParseInt(v))
		ret := model.DB().Find(&crontabTask, "id=?", libs.ParseInt(v))
		if ret.Error != nil {
			*ok = false
		} else {
			globalCrontab.stop(&crontabTask)
		}
	}

	return nil
}

func (t *CrontabTask) StopAll(args []string, ok *bool) error {
	*ok = true
	for _, v := range args {
		var crontabTask model.CrontabTask
		ret := model.DB().Find(&crontabTask, "id", libs.ParseInt(v))
		if ret.Error != nil {
			*ok = false
		} else {
			globalCrontab.stop(&crontabTask)
		}
	}
	return nil
}

func (t *CrontabTask) Delete(args string, ok *bool) error {
	*ok = true
	ids := strings.Split(args, ",")
	for _, v := range ids {
		var crontabTask model.CrontabTask
		ret := model.DB().Find(&crontabTask, "id=?", libs.ParseInt(v))
		if ret.Error != nil {
			*ok = false
		} else {
			globalCrontab.delete(&crontabTask)
		}
	}

	return nil
}

func (t *CrontabTask) Kill(args string, ok *bool) error {

	var crontabTask model.CrontabTask
	ret := model.DB().Find(&crontabTask, "id=?", libs.ParseInt(args))
	if ret.Error != nil {
		*ok = false
	} else {
		globalCrontab.kill(&crontabTask)
	}

	return nil
}

func (t *CrontabTask) QuickStart(args string, reply *[]byte) error {

	var crontabTask model.CrontabTask
	ret := model.DB().Find(&crontabTask, "id=?", libs.ParseInt(args))

	if ret.Error == nil {
		globalCrontab.quickStart(&crontabTask, reply)
	} else {
		*reply = []byte("failed to start")
	}
	return nil

}

func (t *CrontabTask) Log(args proto.SearchLog, reply *proto.SearchLogResult) error {

	fd := finder.NewFinder(1000000, func(info os.FileInfo) bool {
		basename := filepath.Base(info.Name())
		arr := strings.Split(basename, ".")
		if len(arr) != 2 {
			return false
		}
		if arr[1] == "log" && arr[0] == fmt.Sprint(args.TaskId) {
			return true
		}
		return false
	})

	if args.Date == "" {
		args.Date = time.Now().Format("2006/01/02")
	}

	rootpath := filepath.Join(globalConfig.logPath, "crontab_task", args.Date)
	err := fd.Search(rootpath, args.Pattern, &reply.Content, args.Page, args.Pagesize)
	reply.Total = int(fd.Count())
	return err

}

func (t *CrontabTask) ResolvedDepends(args model.DependsTask, reply *bool) error {

	var err error
	if args.Err != "" {
		err = errors.New(args.Err)
	}

	idArr := strings.Split(args.TaskId, "-")
	globalCrontab.lock.Lock()
	i := uint(libs.ParseInt(idArr[0]))
	if h, ok := globalCrontab.handleMap[i]; ok {
		globalCrontab.lock.Unlock()
		for _, v := range h.taskPool {
			if v.id == idArr[1] {
				for _, v2 := range v.depends {
					if v2.id == args.TaskId {
						v2.dest = args.Dest
						v2.from = args.From
						v2.logContent = args.LogContent
						v2.err = err
						v2.done = true
						*reply = filterDepend(v2)
						return nil
					}
				}
			}
		}
	} else {
		globalCrontab.lock.Unlock()
	}

	log.Printf("resolvedDepends: %s is not exists", args.Name)

	*reply = false
	return nil
}

func (t *CrontabTask) ExecDepend(args model.DependsTask, reply *bool) error {

	globalDepend.Add(&dependScript{
		id:      args.TaskId,
		dest:    args.Dest,
		from:    args.From,
		name:    args.Name,
		command: args.Command,
		args:    args.Args,
	})
	*reply = true
	log.Printf("task %s %s %s add to execution queue ", args.Name, args.Command, args.Args)
	return nil
}