package main

import (
	"encoding/json"
	"github.com/kobiton/bitrise-step-kobiton-execute-test/model"
	"github.com/kobiton/bitrise-step-kobiton-execute-test/utils"
	"log"
	"os"
	"strings"
	"time"
)

const MAX_MS_WAIT_FOR_EXECUTION = 1 * 3600 * 1000 // 1 hour in miliseconds

var jobId = ""
var reportUrl = ""

func main() {
	stepConfig := new(model.StepConfig)
	stepConfig.Init()
	log.Println("nhc step v2")

	var headers = getRequestHeader(stepConfig)

	executorPayload := new(model.ExecutorRequestPayload)
	model.BuildExecutorRequestPayload(executorPayload, stepConfig)
	executorJsonPayload, _ := json.MarshalIndent(executorPayload, "", "   ")
	client := utils.HttpClient()

	var executorUrl = stepConfig.GetExecutorUrl() + "/submit"

	var response = utils.SendRequest(client, "POST", executorUrl, headers, executorJsonPayload)

	jobId = string(response)

	if stepConfig.IsWaitForExecution() {

		log.Printf("Requesting to get logs for job %s", jobId)

		var getJobInfoUrl = stepConfig.GetExecutorUrl() + "/jobs/" + jobId
		var getJobLogUrl = getJobInfoUrl + "/logs?type=" + stepConfig.GetLogType()
		var getReportUrl = getJobInfoUrl + "/report"
		var isTimeout = false

		ticker := time.NewTicker(30 * time.Second)
		var jobResponse model.JobResponse
		var waitingBeginAt = time.Now().UnixNano() / int64(time.Millisecond)

		for range ticker.C {
			var response = utils.SendRequest(client, "GET", getJobInfoUrl, headers, nil)
			json.Unmarshal(response, &jobResponse)
			log.Println("Job Status: ", jobResponse.Status)

			if jobResponse.Status == "COMPLETED" || jobResponse.Status == "FAILED" {
				log.Printf("Job ID %s is finish with status: %s", jobId, jobResponse.Status)
				break
			} else {
				var currentTime = time.Now().UnixNano() / int64(time.Millisecond)

				if currentTime-waitingBeginAt >= MAX_MS_WAIT_FOR_EXECUTION {
					isTimeout = true
					break
				}
			}
		}
		defer ticker.Stop()

		if isTimeout {
			log.Println("==============================================================================")
			log.Println("Execution has reached maximum waiting time")
		} else {
			var logResponse = utils.SendRequest(client, "GET", getJobLogUrl, headers, nil)

			log.Println("==============================================================================")
			log.Println(string(logResponse))

			var reportResponse = utils.SendRequest(client, "GET", getReportUrl, headers, nil)
			reportUrl = string(reportResponse)
		}

		// TODO check timeout
		if stepConfig.GetScriptlessAutomation() {
			runScriptless(stepConfig)
		}
	}

	log.Println("==============================================================================")

	if jobId != "" {
		log.Println("Job ID: ", jobId)
	}

	if reportUrl != "" {
		log.Println("Report URL: ", reportUrl)
	}
	//
	// --- Step Outputs: Export Environment Variables for other Steps:
	// You can export Environment Variables for other Steps with
	//  envman, which is automatically installed by `bitrise setup`.
	// A very simple example:
	utils.ExposeEnv("JOB_ID", jobId)
	utils.ExposeEnv("REPORT_URL", reportUrl)
	// You can find more usage examples on envman's GitHub page
	//  at: https://github.com/bitrise-io/envman

	//
	// --- Exit codes:
	// The exit code of your Step is very important. If you return
	//  with a 0 exit code `bitrise` will register your Step as "successful".
	// Any non zero exit code will be registered as "failed" by `bitrise`.
	os.Exit(0)
}

func runScriptless(stepConfig *model.StepConfig) {
	log.Println("Check scriptless status...")

	var isTimeout = false
	var scriptlessResponse model.ScriptlessStatusResponse
	var waitingBeginAt = time.Now().UnixNano() / int64(time.Millisecond)
	var statusUrl = stepConfig.GetExecutorUrl() + "/jobs/" + jobId + "/scriptless/status"
	scriptlessTicker := time.NewTicker(30 * time.Second)
	client := utils.HttpClient()
	var headers = getRequestHeader(stepConfig)

	for range scriptlessTicker.C {
		var response = utils.SendRequest(client, "GET", statusUrl, headers, nil)
		json.Unmarshal(response, &scriptlessResponse)
		if len(scriptlessResponse.Messages) > 0 {
			for _, message := range scriptlessResponse.Messages {
				log.Println(message)
			}
		}

		if scriptlessResponse.Status == "COMPLETED" {
			break
		} else {
			var currentTime = time.Now().UnixNano() / int64(time.Millisecond)

			if currentTime-waitingBeginAt >= stepConfig.GetScriptlessTimeout()*1000 {
				isTimeout = true
				break
			}
		}
	}

	defer scriptlessTicker.Stop()

	if isTimeout {
		log.Println("Scriptless is timeout")
	} else {
		log.Println("Scriptless is completed, copy report to bitrise")
		fileUrl := "https://golangcode.com/logo.svg"
		err := DownloadFile(os.Getenv("BITRISE_DEPLOY_DIR")+"/scriptless-report.html", fileUrl)
		if err == nil {
			log.Println("Upload report success")
		} else {
			log.Println("Upload report failed")
			log.Println(err)
		}
	}
}

func getRequestHeader(stepConfig *model.StepConfig) map[string]string {
	var executorBasicAuth = strings.Join([]string{stepConfig.GetExecutorUsername(), stepConfig.GetExecutorPassword()}, ":")
	var executorBasicAuthEncoded = utils.Base64Encode(executorBasicAuth)

	var headers = map[string]string{}
	headers["x-kobiton-credential-username"] = stepConfig.GetKobiUsername()
	headers["x-kobiton-credential-api-key"] = stepConfig.GetKobiPassword()
	headers["authorization"] = "Basic " + executorBasicAuthEncoded
	headers["content-type"] = "application/json"
	headers["accept"] = "application/json"

	return headers
}
