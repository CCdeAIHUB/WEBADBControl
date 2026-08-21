package api

import (
	"net/http"

	"github.com/CCdeAIHUB/WEBADBControl/server/internal/automation"
)

func (s *Server) listTasks(writer http.ResponseWriter, request *http.Request) {
	tasks, err := s.automation.List(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeData(writer, http.StatusOK, tasks)
}

func (s *Server) saveTask(writer http.ResponseWriter, request *http.Request) {
	var task automation.Task
	if !decodeJSON(writer, request, &task) {
		return
	}
	if id := request.PathValue("id"); id != "" {
		task.ID = id
	}
	saved, err := s.automation.Save(request.Context(), task)
	if err != nil {
		writeError(writer, http.StatusBadRequest, err)
		return
	}
	writeData(writer, http.StatusOK, saved)
}

func (s *Server) deleteTask(writer http.ResponseWriter, request *http.Request) {
	if err := s.automation.Delete(request.Context(), request.PathValue("id")); err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) runTask(writer http.ResponseWriter, request *http.Request) {
	run, err := s.automation.Run(request.Context(), request.PathValue("id"))
	if err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeData(writer, http.StatusAccepted, run)
}

func (s *Server) listRuns(writer http.ResponseWriter, request *http.Request) {
	runs, err := s.automation.Runs(request.Context())
	if err != nil {
		writeError(writer, http.StatusInternalServerError, err)
		return
	}
	writeData(writer, http.StatusOK, runs)
}

func (s *Server) controlRun(writer http.ResponseWriter, request *http.Request) {
	if err := s.automation.Control(request.PathValue("id"), request.PathValue("operation")); err != nil {
		writeError(writer, http.StatusConflict, err)
		return
	}
	writeData(writer, http.StatusAccepted, map[string]bool{"accepted": true})
}
