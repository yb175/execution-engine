package main

import (
	"errors"
	"execution-engine/helpers"
	"execution-engine/internal/execution"
	"execution-engine/internal/handlers"
	"execution-engine/internal/workspace"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type testRunner interface {
	RunTests(attemptID string) (*execution.TestResult, error)
	StartServer(attemptID string) (string, error)
	StopServer(attemptID string) error
}

func newRouter(fsManager *workspace.FsManager, challengePath string, runner testRunner) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	attemptService := execution.NewService(fsManager, challengePath)
	attemptHandler := handlers.NewAttemptHandlers(attemptService)

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Server is running"})
	})
	r.POST("/attempts", func(c *gin.Context) {
		attemptID, err := attemptHandler.CreateAttempt()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"attemptID": attemptID})
	})
	r.DELETE("/attempts", func(c *gin.Context) {
		attemptID, err := helpers.ReqBody(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := attemptHandler.DeleteAttempt(attemptID); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			} else if attemptID == "" || attemptID == "." || attemptID == ".." || filepath.Base(attemptID) != attemptID {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": "workspace deleted successfully", "attemptId": attemptID})
	})
	r.POST("/run-tests", func(c *gin.Context) {
		attemptID, err := helpers.ReqBody(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		results, err := runner.RunTests(attemptID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			} else if attemptID == "" || attemptID == "." || attemptID == ".." || filepath.Base(attemptID) != attemptID {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"results": results})
	})

	r.POST("/start-server", func(c *gin.Context) {
		attemptID, err := helpers.ReqBody(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		results, err := runner.StartServer(attemptID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			} else if attemptID == "" || attemptID == "." || attemptID == ".." || filepath.Base(attemptID) != attemptID {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"results": results})

	})

	r.POST("/stop-server", func(c *gin.Context) {
		attemptID, err := helpers.ReqBody(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		err = runner.StopServer(attemptID)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, os.ErrNotExist) {
				status = http.StatusNotFound
			} else if attemptID == "" || attemptID == "." || attemptID == ".." || filepath.Base(attemptID) != attemptID {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": "server stopped", "attemptId": attemptID})
	})
	return r
}

func main() {
	fsManager := workspace.NewFSManager("./workspaces")
	runnner := execution.NewRunner(fsManager)
	r := newRouter(fsManager, "./challenge/broken-repo", runnner)
	if err := r.Run(":4010"); err != nil {
		log.Fatalf("failed to run server: %v", err)
	}
}
