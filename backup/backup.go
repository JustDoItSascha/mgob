package backup

import (
	"fmt"
	"github.com/codeskyblue/go-sh"
	"github.com/pkg/errors"
	"github.com/stefanprodan/mgob/config"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Run(plan config.Plan, tmpPath string, storagePath string) (Result, error) {
	t1 := time.Now()
	planDir := fmt.Sprintf("%v/%v", storagePath, plan.Name)

	archive, log, err := dump(plan, tmpPath, t1.UTC())
	res := Result{
		Plan: plan.Name,
		Timestamp: t1.UTC(),
		Status:    500,
	}
	_, res.Name = filepath.Split(archive)

	if err != nil {
		return res, err
	}

	err = sh.Command("mkdir", "-p", planDir).Run()
	if err != nil {
		return res, errors.Wrapf(err, "creating dir %v in %v failed", plan.Name, storagePath)
	}

	fi, err := os.Stat(archive)
	if err != nil {
		return res, errors.Wrapf(err, "stat file %v failed", archive)
	}
	res.Size = fi.Size()

	err = sh.Command("mv", archive, planDir).Run()
	if err != nil {
		return res, errors.Wrapf(err, "moving file from %v to %v failed", archive, planDir)
	}

	err = sh.Command("mv", log, planDir).Run()
	if err != nil {
		return res, errors.Wrapf(err, "moving file from %v to %v failed", log, planDir)
	}

	if plan.Scheduler.Retention > 0 {
		err = applyRetention(planDir, plan.Scheduler.Retention)
		if err != nil {
			return res, errors.Wrap(err, "retention job failed")
		}
	}

	t2 := time.Now()
	res.Status = 200
	res.Duration = t2.Sub(t1)
	return res, nil
}

func dump(plan config.Plan, tmpPath string, ts time.Time) (string, string, error) {

	archive := fmt.Sprintf("%v/%v-%v.gz", tmpPath, plan.Name, ts.Unix())
	log := fmt.Sprintf("%v/%v-%v.log", tmpPath, plan.Name, ts.Unix())

	dump := fmt.Sprintf("mongodump --archive=%v --gzip --host %v --port %v --db %v ",
		archive, plan.Target.Host, plan.Target.Port, plan.Target.Database)
	if plan.Target.Username != "" && plan.Target.Password != "" {
		dump += fmt.Sprintf("-u %v -p %v", plan.Target.Username, plan.Target.Password)
	}

	output, err := sh.Command("/bin/sh", "-c", dump).SetTimeout(time.Duration(plan.Scheduler.Timeout) * time.Minute).CombinedOutput()
	if err != nil {
		ex := ""
		if len(output) > 0 {
			ex = strings.Replace(string(output), "\n", " ", -1)
		}
		return "", "", errors.Wrapf(err, "mongodump log %v", ex)
	}
	logToFile(log, output)

	return archive, log, nil
}

func logToFile(file string, data []byte) error {
	if len(data) > 0 {
		err := ioutil.WriteFile(file, data, 0644)
		if err != nil {
			return errors.Wrapf(err, "writing log %v failed", file)
		}
	}

	return nil
}

func applyRetention(path string, retention int) error {
	gz := fmt.Sprintf("cd %v && rm -f $(ls -1t *.gz | tail -n +%v)", path, retention+1)
	err := sh.Command("/bin/sh", "-c", gz).Run()
	if err != nil {
		return errors.Wrapf(err, "removing old gz files from %v failed", path)
	}

	log := fmt.Sprintf("cd %v && rm -f $(ls -1t *.log | tail -n +%v)", path, retention+1)
	err = sh.Command("/bin/sh", "-c", log).Run()
	if err != nil {
		return errors.Wrapf(err, "removing old log files from %v failed", path)
	}

	return nil
}

func CheckMongodump() (string, error) {
	output, err := sh.Command("/bin/sh", "-c", "mongodump --version").CombinedOutput()
	if err != nil {
		ex := ""
		if len(output) > 0 {
			ex = strings.Replace(string(output), "\n", " ", -1)
		}
		return "", errors.Wrapf(err, "mongodump failed %v", ex)
	}

	return strings.Replace(string(output), "\n", " ", -1), nil
}