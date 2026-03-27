package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	sigYaml "sigs.k8s.io/yaml"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
	"github.com/Al-Pragliola/ai-pipelines/internal/issuehistory"
	"github.com/Al-Pragliola/ai-pipelines/internal/trigger"
)

type Server struct {
	k8s        client.Client
	clientset  *kubernetes.Clientset
	restConfig *rest.Config
	frontend   fs.FS
	history    *issuehistory.Store
	logFile    string
}

func NewServer(frontend fs.FS, history *issuehistory.Store, logFile string) (*Server, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	addScheme := aiv1alpha1.SchemeBuilder.SchemeBuilder.AddToScheme
	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("creating k8s client: %w", err)
	}
	if err := addScheme(k8sClient.Scheme()); err != nil {
		return nil, fmt.Errorf("adding scheme: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating clientset: %w", err)
	}

	return &Server{k8s: k8sClient, clientset: clientset, restConfig: cfg, frontend: frontend, history: history, logFile: logFile}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("GET /api/pipelines", s.handleListPipelines)
	mux.HandleFunc("POST /api/pipelines", s.handleCreatePipeline)
	mux.HandleFunc("GET /api/pipelines/{namespace}/{name}/yaml", s.handleGetPipelineYAML)
	mux.HandleFunc("GET /api/pipelines/{namespace}/{name}", s.handleGetPipeline)
	mux.HandleFunc("PUT /api/pipelines/{namespace}/{name}", s.handleUpdatePipeline)
	mux.HandleFunc("DELETE /api/pipelines/{namespace}/{name}", s.handleDeletePipeline)
	mux.HandleFunc("GET /api/pipelines/{namespace}/{name}/runs", s.handleListRuns)
	mux.HandleFunc("GET /api/pipelines/{namespace}/{name}/repos", s.handleListRepos)
	mux.HandleFunc("GET /api/runs/{namespace}/{name}", s.handleGetRun)
	mux.HandleFunc("GET /api/runs/{namespace}/{name}/steps/{step}/logs", s.handleGetLogs)
	mux.HandleFunc("POST /api/runs/{namespace}/{name}/select-repo", s.handleSelectRepo)
	mux.HandleFunc("GET /api/history", s.handleListHistory)
	mux.HandleFunc("POST /api/history", s.handleSkipIssue)
	mux.HandleFunc("DELETE /api/history/{namespace}/{pipeline}/{issueKey}", s.handleDeleteHistory)
	mux.HandleFunc("GET /api/pipelines/{namespace}/{name}/pending-issues", s.handlePendingIssues)
	mux.HandleFunc("POST /api/runs/{namespace}/{name}/retry", s.handleRetryRun)
	mux.HandleFunc("POST /api/runs/{namespace}/{name}/stop", s.handleStopRun)
	mux.HandleFunc("POST /api/runs/{namespace}/{name}/approve", s.handleApproveStep)
	mux.HandleFunc("GET /api/runs/{namespace}/{name}/diff", s.handleGetDiff)
	mux.HandleFunc("POST /api/runs/{namespace}/{name}/diff/refresh", s.handleRefreshDiff)
	mux.HandleFunc("POST /api/runs/{namespace}/{name}/chat", s.handleStartChat)
	mux.HandleFunc("POST /api/runs/{namespace}/{name}/chat/message", s.handleChatMessage)
	mux.HandleFunc("GET /api/runs/{namespace}/{name}/chat/diff", s.handleChatDiff)
	mux.HandleFunc("DELETE /api/runs/{namespace}/{name}/chat", s.handleStopChat)
	mux.HandleFunc("DELETE /api/runs/{namespace}/{name}", s.handleDeleteRun)
	mux.HandleFunc("GET /api/operator/logs", s.handleOperatorLogs)

	// Frontend — serve embedded SPA
	if s.frontend != nil {
		fileServer := http.FileServer(http.FS(s.frontend))
		mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			// Try to serve the file directly
			path := r.URL.Path
			if path == "/" {
				path = "index.html"
			} else {
				path = strings.TrimPrefix(path, "/")
			}
			if _, err := fs.Stat(s.frontend, path); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
			// SPA fallback — serve index.html for client-side routing
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
		})
	}

	return mux
}

// --- API types ---

type pipelineResponse struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	TriggerType  string `json:"triggerType"`
	TriggerInfo  string `json:"triggerInfo"`
	TriggerJQL   string `json:"triggerJql,omitempty"`
	PollInterval string `json:"pollInterval"`
	ActiveRuns   int    `json:"activeRuns"`
	TotalRuns    int    `json:"totalRuns"`
}

type runResponse struct {
	Name            string                   `json:"name"`
	Namespace       string                   `json:"namespace"`
	Pipeline        string                   `json:"pipeline"`
	IssueNumber     int                      `json:"issueNumber"`
	IssueKey        string                   `json:"issueKey"`
	IssueTitle      string                   `json:"issueTitle"`
	Phase           string                   `json:"phase"`
	CurrentStep     string                   `json:"currentStep"`
	Branch          string                   `json:"branch"`
	WaitingFor      string                   `json:"waitingFor,omitempty"`
	DiffJobName     string                   `json:"diffJobName,omitempty"`
	ChatPodName     string                   `json:"chatPodName,omitempty"`
	ResolvedRepo    *aiv1alpha1.SelectedRepo `json:"resolvedRepo,omitempty"`
	TriageResult    *aiv1alpha1.TriageResult `json:"triageResult,omitempty"`
	StartedAt       *string                  `json:"startedAt"`
	FinishedAt      *string                  `json:"finishedAt"`
	DurationSeconds *int64                   `json:"durationSeconds,omitempty"`
	Steps           []stepResponse           `json:"steps"`
}

type stepResponse struct {
	Name            string  `json:"name"`
	Type            string  `json:"type"`
	Phase           string  `json:"phase"`
	StartedAt       *string `json:"startedAt"`
	FinishedAt      *string `json:"finishedAt"`
	DurationSeconds *int64  `json:"durationSeconds,omitempty"`
	JobName         string  `json:"jobName"`
	Attempt         int     `json:"attempt"`
	Message         string  `json:"message"`
}

// --- Handlers ---

func (s *Server) handleListPipelines(w http.ResponseWriter, r *http.Request) {
	var pipelines aiv1alpha1.PipelineList
	if err := s.k8s.List(r.Context(), &pipelines); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Count runs per pipeline
	var allRuns aiv1alpha1.PipelineRunList
	_ = s.k8s.List(r.Context(), &allRuns)

	runCounts := map[string][2]int{} // [active, total]
	for _, run := range allRuns.Items {
		key := run.Namespace + "/" + run.Spec.PipelineRef
		counts := runCounts[key]
		counts[1]++
		if run.Status.Phase == aiv1alpha1.PipelineRunPhasePending || run.Status.Phase == aiv1alpha1.PipelineRunPhaseRunning || run.Status.Phase == aiv1alpha1.PipelineRunPhaseWaitingForInput {
			counts[0]++
		}
		runCounts[key] = counts
	}

	out := make([]pipelineResponse, 0, len(pipelines.Items))
	for _, p := range pipelines.Items {
		counts := runCounts[p.Namespace+"/"+p.Name]
		resp := pipelineResponse{
			Name:       p.Name,
			Namespace:  p.Namespace,
			ActiveRuns: counts[0],
			TotalRuns:  counts[1],
		}
		switch {
		case p.Spec.Trigger.GitHub != nil:
			resp.TriggerType = "GitHub"
			resp.TriggerInfo = fmt.Sprintf("%s/%s (assignee: %s)", p.Spec.Trigger.GitHub.Owner, p.Spec.Trigger.GitHub.Repo, p.Spec.Trigger.GitHub.Assignee)
			resp.PollInterval = p.Spec.Trigger.GitHub.PollInterval
		case p.Spec.Trigger.Jira != nil:
			resp.TriggerType = "Jira"
			resp.TriggerInfo = p.Spec.Trigger.Jira.URL
			resp.TriggerJQL = p.Spec.Trigger.Jira.JQL
			resp.PollInterval = p.Spec.Trigger.Jira.PollInterval
		}
		out = append(out, resp)
	}

	writeJSON(w, out)
}

func (s *Server) handleGetPipeline(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pipeline); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, struct {
		Name      string                  `json:"name"`
		Namespace string                  `json:"namespace"`
		Spec      aiv1alpha1.PipelineSpec `json:"spec"`
	}{
		Name:      pipeline.Name,
		Namespace: pipeline.Namespace,
		Spec:      pipeline.Spec,
	})
}

func (s *Server) handleGetPipelineYAML(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pipeline); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Build a clean CR representation (no status, no managed fields)
	cr := map[string]any{
		"apiVersion": aiv1alpha1.GroupVersion.String(),
		"kind":       "Pipeline",
		"metadata": map[string]any{
			"name":      pipeline.Name,
			"namespace": pipeline.Namespace,
		},
		"spec": pipeline.Spec,
	}

	out, err := sigYaml.Marshal(cr)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to marshal YAML: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(out)
}

func (s *Server) handleCreatePipeline(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string                  `json:"name"`
		Namespace string                  `json:"namespace"`
		Spec      aiv1alpha1.PipelineSpec `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if body.Namespace == "" {
		body.Namespace = "default"
	}

	pipeline := &aiv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      body.Name,
			Namespace: body.Namespace,
		},
		Spec: body.Spec,
	}

	if err := s.k8s.Create(r.Context(), pipeline); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "already exists") {
			status = http.StatusConflict
		}
		http.Error(w, fmt.Sprintf("failed to create pipeline: %v", err), status)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, map[string]string{"status": "created", "name": body.Name, "namespace": body.Namespace})
}

func (s *Server) handleUpdatePipeline(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var body struct {
		Spec aiv1alpha1.PipelineSpec `json:"spec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pipeline); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	pipeline.Spec = body.Spec
	if err := s.k8s.Update(r.Context(), &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("failed to update pipeline: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "updated"})
}

func (s *Server) handleDeletePipeline(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pipeline); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if err := s.k8s.Delete(r.Context(), &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("failed to delete pipeline: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var runs aiv1alpha1.PipelineRunList
	if err := s.k8s.List(r.Context(), &runs, client.InNamespace(namespace)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	out := make([]runResponse, 0)
	for _, run := range runs.Items {
		if run.Spec.PipelineRef != name {
			continue
		}
		out = append(out, toRunResponse(&run))
	}

	// Sort by creation time, newest first
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt == nil {
			return false
		}
		if out[j].StartedAt == nil {
			return true
		}
		return *out[i].StartedAt > *out[j].StartedAt
	})

	writeJSON(w, out)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, toRunResponse(&run))
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")
	step := r.PathValue("step")

	// Get the run to find the job name for this step
	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var jobName string
	var stepType string
	for _, s := range run.Status.Steps {
		if s.Name == step && s.JobName != "" {
			jobName = s.JobName
			stepType = s.Type
		}
	}
	if jobName == "" {
		http.Error(w, "step not found or no job", http.StatusNotFound)
		return
	}

	// Find pod for this job
	pods, err := s.clientset.CoreV1().Pods(namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil || len(pods.Items) == 0 {
		http.Error(w, "no pod found for job", http.StatusNotFound)
		return
	}

	pod := pods.Items[0]
	logOpts := &corev1.PodLogOptions{}

	// Pick the right container for logs based on step type:
	// - triage: AI runs in init container "ai", main is just "reader"
	// - ai/shell: main container is "ai" or "shell"
	// Always set container explicitly to avoid ambiguity when sidecars (DinD) are present.
	switch stepType {
	case "triage":
		logOpts.Container = "ai"
		// If the AI init container hasn't produced logs yet, fall back to reader
		for _, ic := range pod.Status.InitContainerStatuses {
			if ic.Name == "ai" && ic.State.Waiting != nil {
				logOpts.Container = ""
			}
		}
	case "ai":
		logOpts.Container = "ai"
	case "shell":
		logOpts.Container = "shell"
	default:
		// For git-checkout, git-push, etc. — use the first main container
		if len(pod.Spec.Containers) > 0 {
			logOpts.Container = pod.Spec.Containers[0].Name
		}
	}

	logStream, err := s.clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, logOpts).Stream(context.Background())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get logs: %v", err), http.StatusInternalServerError)
		return
	}
	defer logStream.Close() //nolint:errcheck

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, logStream)
}

func (s *Server) handleListRepos(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pipeline); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	type repoEntry struct {
		Owner       string `json:"owner"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
	}

	repos := make([]repoEntry, 0, len(pipeline.Spec.Repos))
	for _, c := range pipeline.Spec.Repos {
		repos = append(repos, repoEntry{
			Owner:       c.Owner,
			Name:        c.Name,
			Description: c.Description,
		})
	}
	if pipeline.Spec.Repo != nil && len(repos) == 0 {
		repos = append(repos, repoEntry{
			Owner: pipeline.Spec.Repo.Owner,
			Name:  pipeline.Spec.Repo.Name,
		})
	}

	writeJSON(w, repos)
}

func (s *Server) handleSelectRepo(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var body struct {
		Owner string `json:"owner"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.Owner == "" || body.Name == "" {
		http.Error(w, "owner and name are required", http.StatusBadRequest)
		return
	}

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if run.Status.Phase != aiv1alpha1.PipelineRunPhaseWaitingForInput {
		http.Error(w, "run is not waiting for input", http.StatusConflict)
		return
	}

	run.Spec.SelectedRepo = &aiv1alpha1.SelectedRepo{
		Owner: body.Owner,
		Name:  body.Name,
	}
	if err := s.k8s.Update(r.Context(), &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to update run: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleOperatorLogs(w http.ResponseWriter, r *http.Request) {
	var tailLines int64 = 500
	if n := r.URL.Query().Get("lines"); n != "" {
		if parsed, err := strconv.ParseInt(n, 10, 64); err == nil && parsed > 0 {
			tailLines = parsed
		}
	}

	// Try pod logs first (in-cluster mode)
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{
		LabelSelector: "control-plane=controller-manager",
	})
	if err == nil && len(pods.Items) > 0 {
		pod := pods.Items[0]
		logStream, err := s.clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
			TailLines: &tailLines,
		}).Stream(r.Context())
		if err == nil {
			defer logStream.Close() //nolint:errcheck
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.Copy(w, logStream)
			return
		}
	}

	// Fallback: read from log file (local dev mode)
	if s.logFile != "" {
		s.serveLogFile(w, tailLines)
		return
	}

	http.Error(w, "no controller pod found and no log file configured", http.StatusNotFound)
}

func (s *Server) serveLogFile(w http.ResponseWriter, tailLines int64) {
	data, err := os.ReadFile(s.logFile)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read log file: %v", err), http.StatusInternalServerError)
		return
	}

	lines := strings.Split(string(data), "\n")
	if int64(len(lines)) > tailLines {
		lines = lines[int64(len(lines))-tailLines:]
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(strings.Join(lines, "\n")))
}

func (s *Server) handleListHistory(w http.ResponseWriter, r *http.Request) {
	if s.history == nil {
		writeJSON(w, []struct{}{})
		return
	}
	records, err := s.history.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []issuehistory.Record{}
	}
	writeJSON(w, records)
}

func (s *Server) handleDeleteHistory(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	pipeline := r.PathValue("pipeline")
	issueKey := r.PathValue("issueKey")

	// Delete the skip PipelineRun CR if one exists (so the poller can pick up the issue again)
	var runs aiv1alpha1.PipelineRunList
	if err := s.k8s.List(r.Context(), &runs, client.InNamespace(namespace), client.MatchingLabels{
		"ai.aipipelines.io/pipeline":  pipeline,
		"ai.aipipelines.io/issue-key": sanitizeLabelValue(issueKey),
	}); err == nil {
		for i := range runs.Items {
			if runs.Items[i].Annotations["ai.aipipelines.io/skipped"] == "true" {
				_ = s.k8s.Delete(r.Context(), &runs.Items[i])
			}
		}
	}

	// Delete from local history DB
	if s.history != nil {
		if err := s.history.Delete(r.Context(), namespace, pipeline, issueKey); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleSkipIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PipelineNamespace string `json:"pipelineNamespace"`
		PipelineName      string `json:"pipelineName"`
		IssueKey          string `json:"issueKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.PipelineName == "" || body.IssueKey == "" {
		http.Error(w, "pipelineName and issueKey are required", http.StatusBadRequest)
		return
	}
	if body.PipelineNamespace == "" {
		body.PipelineNamespace = "default"
	}

	// Validate pipeline exists
	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: body.PipelineNamespace, Name: body.PipelineName}, &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("pipeline not found: %v", err), http.StatusNotFound)
		return
	}

	// Create a PipelineRun CR with the skipped annotation.
	// The controller's poller dedup (layer 1) checks for PipelineRun CRs with
	// matching labels — this blocks the issue immediately and works across
	// separate processes/pods (no shared SQLite needed).
	// The PipelineRunReconciler sees the annotation and marks it Stopped.
	skipRun := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-skip-", body.PipelineName),
			Namespace:    body.PipelineNamespace,
			Labels: map[string]string{
				"ai.aipipelines.io/pipeline":  body.PipelineName,
				"ai.aipipelines.io/issue-key": sanitizeLabelValue(body.IssueKey),
			},
			Annotations: map[string]string{
				"ai.aipipelines.io/skipped": "true",
			},
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: body.PipelineName,
			IssueKey:    body.IssueKey,
		},
	}

	if err := controllerutil.SetControllerReference(&pipeline, skipRun, s.k8s.Scheme()); err != nil {
		http.Error(w, fmt.Sprintf("failed to set owner reference: %v", err), http.StatusInternalServerError)
		return
	}

	if err := s.k8s.Create(r.Context(), skipRun); err != nil {
		http.Error(w, fmt.Sprintf("failed to create skip record: %v", err), http.StatusInternalServerError)
		return
	}

	// Also write to local history DB for dashboard display
	if s.history != nil {
		_ = s.history.MarkCompleted(r.Context(), issuehistory.Record{
			PipelineNamespace: body.PipelineNamespace,
			PipelineName:      body.PipelineName,
			IssueKey:          body.IssueKey,
			Phase:             "Skipped",
			RunName:           skipRun.Name,
			CompletedAt:       time.Now(),
		})
	}

	writeJSON(w, map[string]string{"status": "skipped", "runName": skipRun.Name})
}

func (s *Server) handlePendingIssues(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("pipeline not found: %v", err), http.StatusNotFound)
		return
	}

	// Read trigger credentials
	var secretRef aiv1alpha1.SecretKeyRef
	switch {
	case pipeline.Spec.Trigger.GitHub != nil:
		secretRef = pipeline.Spec.Trigger.GitHub.SecretRef
	case pipeline.Spec.Trigger.Jira != nil:
		secretRef = pipeline.Spec.Trigger.Jira.SecretRef
	default:
		writeJSON(w, []struct{}{})
		return
	}

	key := secretRef.Key
	if key == "" {
		key = "token"
	}
	secret, err := s.clientset.CoreV1().Secrets(namespace).Get(r.Context(), secretRef.Name, metav1.GetOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to read trigger secret: %v", err), http.StatusInternalServerError)
		return
	}
	token := string(secret.Data[key])

	var jiraEmail string
	if pipeline.Spec.Trigger.Jira != nil {
		if v, ok := secret.Data["email"]; ok {
			jiraEmail = string(v)
		}
	}

	// Fetch issues from the trigger source
	var issues []trigger.Issue
	switch {
	case pipeline.Spec.Trigger.GitHub != nil:
		issues, err = trigger.FetchGitHubIssues(r.Context(), pipeline.Spec.Trigger.GitHub, token)
	case pipeline.Spec.Trigger.Jira != nil:
		issues, err = trigger.FetchJiraIssues(r.Context(), pipeline.Spec.Trigger.Jira, token, jiraEmail)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to fetch issues: %v", err), http.StatusInternalServerError)
		return
	}

	// Filter out issues that are already completed or have active PipelineRuns
	type pendingIssue struct {
		Key   string `json:"key"`
		Title string `json:"title"`
	}
	var pending []pendingIssue

	for _, issue := range issues {
		// Check history (completed/skipped)
		if s.history != nil {
			completed, err := s.history.IsCompleted(r.Context(), namespace, name, issue.Key)
			if err == nil && completed {
				continue
			}
		}

		// Check active PipelineRun CRs
		var runs aiv1alpha1.PipelineRunList
		if err := s.k8s.List(r.Context(), &runs, client.InNamespace(namespace), client.MatchingLabels{
			"ai.aipipelines.io/pipeline":  name,
			"ai.aipipelines.io/issue-key": sanitizeLabelValue(issue.Key),
		}); err == nil && len(runs.Items) > 0 {
			continue
		}

		pending = append(pending, pendingIssue{Key: issue.Key, Title: issue.Title})
	}

	if pending == nil {
		pending = []pendingIssue{}
	}
	writeJSON(w, pending)
}

func (s *Server) handleRetryRun(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Look up the parent Pipeline for owner reference
	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: run.Spec.PipelineRef}, &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("parent pipeline not found: %v", err), http.StatusNotFound)
		return
	}

	// Clear history entry so the new run can complete normally
	if s.history != nil {
		issueKey := run.Spec.IssueKey
		if issueKey == "" {
			issueKey = fmt.Sprintf("#%d", run.Spec.IssueNumber)
		}
		_ = s.history.Delete(r.Context(), namespace, run.Spec.PipelineRef, issueKey)
	}

	// Create new PipelineRun with same spec but fresh metadata
	newRun := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", run.Spec.PipelineRef),
			Namespace:    namespace,
			Labels: map[string]string{
				"ai.aipipelines.io/pipeline":  run.Spec.PipelineRef,
				"ai.aipipelines.io/issue-key": sanitizeLabelValue(run.Spec.IssueKey),
			},
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: run.Spec.PipelineRef,
			IssueNumber: run.Spec.IssueNumber,
			IssueKey:    run.Spec.IssueKey,
			IssueTitle:  run.Spec.IssueTitle,
			IssueBody:   run.Spec.IssueBody,
		},
	}

	// Set owner reference to Pipeline
	if err := controllerutil.SetControllerReference(&pipeline, newRun, s.k8s.Scheme()); err != nil {
		http.Error(w, fmt.Sprintf("failed to set owner reference: %v", err), http.StatusInternalServerError)
		return
	}

	if err := s.k8s.Create(r.Context(), newRun); err != nil {
		http.Error(w, fmt.Sprintf("failed to create retry run: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{
		"status":    "created",
		"name":      newRun.Name,
		"namespace": newRun.Namespace,
	})
}

func (s *Server) handleStopRun(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if run.Status.Phase == aiv1alpha1.PipelineRunPhaseSucceeded || run.Status.Phase == aiv1alpha1.PipelineRunPhaseFailed || run.Status.Phase == aiv1alpha1.PipelineRunPhaseStopped {
		http.Error(w, "run is already in a terminal state", http.StatusConflict)
		return
	}

	// Delete the active Job for the current step (if any)
	for _, step := range run.Status.Steps {
		if step.JobName != "" && step.Phase == aiv1alpha1.PipelineRunPhaseRunning {
			propagation := metav1.DeletePropagationBackground
			_ = s.clientset.BatchV1().Jobs(namespace).Delete(r.Context(), step.JobName, metav1.DeleteOptions{
				PropagationPolicy: &propagation,
			})
		}
	}

	// Mark as Stopped
	now := metav1.Now()
	run.Status.Phase = aiv1alpha1.PipelineRunPhaseStopped
	run.Status.FinishedAt = &now
	for i := range run.Status.Steps {
		if run.Status.Steps[i].Phase == aiv1alpha1.PipelineRunPhaseRunning || run.Status.Steps[i].Phase == aiv1alpha1.PipelineRunPhasePending || run.Status.Steps[i].Phase == aiv1alpha1.PipelineRunPhaseInitializing {
			run.Status.Steps[i].Phase = aiv1alpha1.PipelineRunPhaseStopped
			run.Status.Steps[i].Message = "Stopped by user"
			run.Status.Steps[i].FinishedAt = &now
		}
	}

	if err := s.k8s.Status().Update(r.Context(), &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to update run status: %v", err), http.StatusInternalServerError)
		return
	}

	// Record in history
	if s.history != nil {
		issueKey := run.Spec.IssueKey
		if issueKey == "" {
			issueKey = fmt.Sprintf("#%d", run.Spec.IssueNumber)
		}
		completedAt := now.Time
		_ = s.history.MarkCompleted(r.Context(), issuehistory.Record{
			PipelineNamespace: namespace,
			PipelineName:      run.Spec.PipelineRef,
			IssueKey:          issueKey,
			Phase:             "Stopped",
			RunName:           run.Name,
			CompletedAt:       completedAt,
		})
	}

	writeJSON(w, map[string]string{"status": "stopped"})
}

func (s *Server) handleApproveStep(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if run.Status.Phase != aiv1alpha1.PipelineRunPhaseWaitingForInput || run.Status.WaitingFor != "step-approval" {
		http.Error(w, "run is not waiting for step approval", http.StatusConflict)
		return
	}

	// Set the approved step in the spec — controller watches for this
	run.Spec.ApprovedStep = run.Status.CurrentStep
	if err := s.k8s.Update(r.Context(), &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to approve step: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "approved", "step": run.Status.CurrentStep})
}

func (s *Server) handleGetDiff(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if run.Status.DiffJobName == "" {
		http.Error(w, "no diff preview available", http.StatusNotFound)
		return
	}

	pods, err := s.clientset.CoreV1().Pods(namespace).List(r.Context(), metav1.ListOptions{
		LabelSelector: "job-name=" + run.Status.DiffJobName,
	})
	if err != nil || len(pods.Items) == 0 {
		http.Error(w, "diff preview not ready yet", http.StatusNotFound)
		return
	}

	// Check if the diff container is ready (still creating = not ready)
	pod := &pods.Items[0]
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == "diff" && cs.State.Waiting != nil {
			http.Error(w, "diff preview not ready yet", http.StatusNotFound)
			return
		}
	}

	logStream, err := s.clientset.CoreV1().Pods(namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
		Container: "diff",
	}).Stream(r.Context())
	if err != nil {
		// Also treat stream errors as "not ready" so frontend retries
		if strings.Contains(err.Error(), "waiting to start") || strings.Contains(err.Error(), "ContainerCreating") {
			http.Error(w, "diff preview not ready yet", http.StatusNotFound)
			return
		}
		http.Error(w, fmt.Sprintf("failed to get diff logs: %v", err), http.StatusInternalServerError)
		return
	}
	defer logStream.Close() //nolint:errcheck

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, logStream)
}

func (s *Server) handleDeleteRun(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if run.Status.Phase == aiv1alpha1.PipelineRunPhaseDeleting {
		writeJSON(w, map[string]string{"status": "already deleting"})
		return
	}

	// Set phase to Deleting — the controller will clean up jobs, PVC, and delete the CR
	run.Status.Phase = aiv1alpha1.PipelineRunPhaseDeleting
	if err := s.k8s.Status().Update(r.Context(), &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to update run status: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "deleting"})
}

// --- Chat ---

func (s *Server) handleStartChat(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if run.Status.Phase != aiv1alpha1.PipelineRunPhaseWaitingForInput || run.Status.WaitingFor != "step-approval" {
		http.Error(w, "run is not waiting for step approval", http.StatusConflict)
		return
	}

	// Reuse existing chat pod if it's still alive
	podName := run.Name + "-chat"
	if existingPod, err := s.clientset.CoreV1().Pods(namespace).Get(r.Context(), podName, metav1.GetOptions{}); err == nil {
		// Pod exists — make sure status is in sync
		if run.Status.ChatPodName != podName {
			run.Status.ChatPodName = podName
			run.Status.DiffJobName = ""
			_ = s.k8s.Status().Update(r.Context(), &run)
		}
		writeJSON(w, map[string]string{"status": "exists", "podName": existingPod.Name})
		return
	}
	// Pod doesn't exist — clear stale status if needed
	if run.Status.ChatPodName != "" {
		run.Status.ChatPodName = ""
		_ = s.k8s.Status().Update(r.Context(), &run)
	}

	// Fetch parent pipeline for AI config
	var pipeline aiv1alpha1.Pipeline
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: run.Spec.PipelineRef}, &pipeline); err != nil {
		http.Error(w, fmt.Sprintf("parent pipeline not found: %v", err), http.StatusNotFound)
		return
	}

	// Delete diff job to free PVC (must wait for pod termination — RWO PVC)
	diffJobName := run.Name + "-diff-preview"
	propagation := metav1.DeletePropagationForeground
	_ = s.clientset.BatchV1().Jobs(namespace).Delete(r.Context(), diffJobName, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	// Also delete by status name if different
	if run.Status.DiffJobName != "" && run.Status.DiffJobName != diffJobName {
		_ = s.clientset.BatchV1().Jobs(namespace).Delete(r.Context(), run.Status.DiffJobName, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
	}
	// Wait for diff job pods to be gone so PVC is free
	for range 20 {
		pods, err := s.clientset.CoreV1().Pods(namespace).List(r.Context(), metav1.ListOptions{
			LabelSelector: "job-name=" + diffJobName,
		})
		if err != nil || len(pods.Items) == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Build chat pod spec (mirrors configureAIJob in the controller)
	var runAsUser int64 = 1000
	var runAsGroup int64 = 1000
	var runAsNonRoot = true
	var allowPrivEsc = false

	var envVars []corev1.EnvVar
	for k, v := range pipeline.Spec.AI.Env {
		envVars = append(envVars, corev1.EnvVar{Name: k, Value: v})
	}
	envVars = append(envVars, corev1.EnvVar{Name: "HOME", Value: "/tmp"})

	volumeMounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: "/workspace"},
	}
	volumes := []corev1.Volume{
		{
			Name: "workspace",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: run.Status.PVCName,
				},
			},
		},
	}

	if pipeline.Spec.AI.SecretRef != nil {
		mountPath := pipeline.Spec.AI.CredentialsMountPath
		if mountPath == "" {
			mountPath = "/tmp/gcp-creds.json"
		}
		secretKey := pipeline.Spec.AI.SecretRef.Key
		if secretKey == "" {
			secretKey = "token"
		}
		volumes = append(volumes, corev1.Volume{
			Name: "ai-credentials",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: pipeline.Spec.AI.SecretRef.Name,
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "ai-credentials",
			MountPath: mountPath,
			SubPath:   secretKey,
		})
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"ai.aipipelines.io/pipeline-run": run.Name,
				"ai.aipipelines.io/chat":         "true",
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser:    &runAsUser,
				RunAsGroup:   &runAsGroup,
				FSGroup:      &runAsGroup,
				RunAsNonRoot: &runAsNonRoot,
			},
			Volumes: volumes,
			Containers: []corev1.Container{
				{
					Name:            "chat",
					Image:           pipeline.Spec.AI.Image,
					ImagePullPolicy: pipeline.Spec.AI.ImagePullPolicy,
					Command:         []string{"sleep", "infinity"},
					WorkingDir:      "/workspace",
					Env:             envVars,
					VolumeMounts:    volumeMounts,
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: &allowPrivEsc,
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(&run, pod, s.k8s.Scheme()); err != nil {
		http.Error(w, fmt.Sprintf("failed to set owner reference: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := s.clientset.CoreV1().Pods(namespace).Create(r.Context(), pod, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			// Pod appeared between our Get and Create (race) — reuse it
			writeJSON(w, map[string]string{"status": "exists", "podName": podName})
			return
		}
		http.Error(w, fmt.Sprintf("failed to create chat pod: %v", err), http.StatusInternalServerError)
		return
	}

	// Re-fetch run before status update (resourceVersion may be stale after waiting)
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to re-fetch run: %v", err), http.StatusInternalServerError)
		return
	}
	run.Status.ChatPodName = podName
	run.Status.DiffJobName = ""
	if err := s.k8s.Status().Update(r.Context(), &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to update run status: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "created", "podName": podName})
}

// flushWriter wraps an http.ResponseWriter and flushes after every write.
type flushWriter struct {
	w http.ResponseWriter
	f http.Flusher
}

func (fw *flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	fw.f.Flush()
	return n, err
}

func (s *Server) handleChatMessage(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if body.Message == "" {
		http.Error(w, "message is required", http.StatusBadRequest)
		return
	}

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if run.Status.ChatPodName == "" {
		http.Error(w, "no chat session active", http.StatusNotFound)
		return
	}

	// Verify pod is running — clear stale status if pod is gone
	pod, err := s.clientset.CoreV1().Pods(namespace).Get(r.Context(), run.Status.ChatPodName, metav1.GetOptions{})
	if err != nil {
		// Pod is gone — clear stale chatPodName so the user can recreate
		run.Status.ChatPodName = ""
		_ = s.k8s.Status().Update(r.Context(), &run)
		http.Error(w, "chat session expired — please reopen the chat", http.StatusGone)
		return
	}
	if pod.Status.Phase != corev1.PodRunning {
		http.Error(w, fmt.Sprintf("chat pod not ready (phase: %s)", pod.Status.Phase), http.StatusServiceUnavailable)
		return
	}

	// Build exec request — pipe message to claude via stdin.
	// Use a marker file to track whether we've sent a first message (--continue fails with no prior session).
	cmd := []string{"/bin/sh", "-c",
		`if [ -f /tmp/.chat-continued ]; then ` +
			`claude -p --dangerously-skip-permissions --continue --output-format stream-json --verbose; ` +
			`else claude -p --dangerously-skip-permissions --output-format stream-json --verbose; ` +
			`r=$?; touch /tmp/.chat-continued; exit $r; fi`,
	}
	req := s.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(run.Status.ChatPodName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "chat",
			Command:   cmd,
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(s.restConfig, "POST", req.URL())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create executor: %v", err), http.StatusInternalServerError)
		return
	}

	// Set streaming response headers
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	flusher.Flush()

	fw := &flushWriter{w: w, f: flusher}

	err = exec.StreamWithContext(r.Context(), remotecommand.StreamOptions{
		Stdin:  strings.NewReader(body.Message),
		Stdout: fw,
		Stderr: fw,
	})
	if err != nil {
		// If we already started writing, we can't send an HTTP error.
		// Write the error as a JSON line instead.
		_, _ = fmt.Fprintf(fw, "\n{\"type\":\"error\",\"error\":%q}\n", err.Error())
	}
}

func (s *Server) handleStopChat(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if run.Status.ChatPodName != "" {
		_ = s.clientset.CoreV1().Pods(namespace).Delete(r.Context(), run.Status.ChatPodName, metav1.DeleteOptions{})
		// Re-fetch before status update to avoid conflict
		if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		run.Status.ChatPodName = ""
		if err := s.k8s.Status().Update(r.Context(), &run); err != nil {
			http.Error(w, fmt.Sprintf("failed to update run status: %v", err), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]string{"status": "deleted"})
}

func (s *Server) handleChatDiff(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if run.Status.ChatPodName == "" {
		http.Error(w, "no chat session active", http.StatusNotFound)
		return
	}

	// Verify pod is running — clear stale status if pod is gone
	pod, err := s.clientset.CoreV1().Pods(namespace).Get(r.Context(), run.Status.ChatPodName, metav1.GetOptions{})
	if err != nil {
		run.Status.ChatPodName = ""
		_ = s.k8s.Status().Update(r.Context(), &run)
		http.Error(w, "chat pod not found", http.StatusNotFound)
		return
	}
	if pod.Status.Phase != corev1.PodRunning {
		http.Error(w, "chat pod not ready", http.StatusServiceUnavailable)
		return
	}

	// Exec git diff in the chat pod
	req := s.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(run.Status.ChatPodName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "chat",
			Command:   []string{"/bin/sh", "-c", "cd /workspace && git config --global --add safe.directory /workspace && git diff HEAD --no-color -- ':!.test-failures.md'"},
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(s.restConfig, "POST", req.URL())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create executor: %v", err), http.StatusInternalServerError)
		return
	}

	var stdout, stderr bytes.Buffer
	if err := exec.StreamWithContext(r.Context(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		http.Error(w, fmt.Sprintf("git diff failed: %v: %s", err, stderr.String()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(stdout.Bytes())
}

func (s *Server) handleRefreshDiff(w http.ResponseWriter, r *http.Request) {
	namespace := r.PathValue("namespace")
	name := r.PathValue("name")

	var run aiv1alpha1.PipelineRun
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Always try to delete the diff job by canonical name (status may be stale)
	jobName := run.Name + "-diff-preview"
	propagation := metav1.DeletePropagationForeground
	_ = s.clientset.BatchV1().Jobs(namespace).Delete(r.Context(), jobName, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
	// Wait briefly for the old job's pod to be gone so the PVC is free
	for range 10 {
		pods, err := s.clientset.CoreV1().Pods(namespace).List(r.Context(), metav1.ListOptions{
			LabelSelector: "job-name=" + jobName,
		})
		if err != nil || len(pods.Items) == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	// Create new diff job (mirrors createDiffJob in the controller)
	var backoffLimit int32 = 0
	script := `cd /workspace && git config --global --add safe.directory /workspace && git add -A && git diff --cached --no-color HEAD -- ':!.test-failures.md'`

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: namespace,
			Labels: map[string]string{
				"ai.aipipelines.io/pipeline-run": run.Name,
				"ai.aipipelines.io/diff-preview": "true",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: &backoffLimit,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{
							Name: "workspace",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: run.Status.PVCName,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "diff",
							Image:   "alpine/git:latest",
							Command: []string{"/bin/sh", "-c"},
							Args:    []string{script},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: "/workspace"},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(&run, job, s.k8s.Scheme()); err != nil {
		http.Error(w, fmt.Sprintf("failed to set owner reference: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := s.clientset.BatchV1().Jobs(namespace).Create(r.Context(), job, metav1.CreateOptions{}); err != nil {
		http.Error(w, fmt.Sprintf("failed to create diff job: %v", err), http.StatusInternalServerError)
		return
	}

	// Re-fetch before status update to avoid conflict
	if err := s.k8s.Get(r.Context(), client.ObjectKey{Namespace: namespace, Name: name}, &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to re-fetch run: %v", err), http.StatusInternalServerError)
		return
	}
	run.Status.DiffJobName = jobName
	if err := s.k8s.Status().Update(r.Context(), &run); err != nil {
		http.Error(w, fmt.Sprintf("failed to update run status: %v", err), http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]string{"status": "created", "diffJobName": jobName})
}

func sanitizeLabelValue(s string) string {
	s = strings.ReplaceAll(s, "#", "")
	if len(s) > 63 {
		s = s[:63]
	}
	return s
}

// --- Helpers ---

func toRunResponse(run *aiv1alpha1.PipelineRun) runResponse {
	r := runResponse{
		Name:         run.Name,
		Namespace:    run.Namespace,
		Pipeline:     run.Spec.PipelineRef,
		IssueNumber:  run.Spec.IssueNumber,
		IssueKey:     run.Spec.IssueKey,
		IssueTitle:   run.Spec.IssueTitle,
		Phase:        string(run.Status.Phase),
		CurrentStep:  run.Status.CurrentStep,
		Branch:       run.Status.Branch,
		WaitingFor:   run.Status.WaitingFor,
		DiffJobName:  run.Status.DiffJobName,
		ChatPodName:  run.Status.ChatPodName,
		ResolvedRepo: run.Status.ResolvedRepo,
		TriageResult: run.Status.TriageResult,
		Steps:        make([]stepResponse, 0, len(run.Status.Steps)),
	}
	if run.Status.StartedAt != nil {
		t := run.Status.StartedAt.Format(time.RFC3339)
		r.StartedAt = &t
		end := time.Now()
		if run.Status.FinishedAt != nil {
			end = run.Status.FinishedAt.Time
		}
		d := max(int64(end.Sub(run.Status.StartedAt.Time).Seconds()), 0)
		r.DurationSeconds = &d
	}
	if run.Status.FinishedAt != nil {
		t := run.Status.FinishedAt.Format(time.RFC3339)
		r.FinishedAt = &t
	}
	for _, s := range run.Status.Steps {
		sr := stepResponse{
			Name:    s.Name,
			Type:    s.Type,
			Phase:   string(s.Phase),
			JobName: s.JobName,
			Attempt: s.Attempt,
			Message: s.Message,
		}
		if s.StartedAt != nil {
			t := s.StartedAt.Format(time.RFC3339)
			sr.StartedAt = &t
			end := time.Now()
			if s.FinishedAt != nil {
				end = s.FinishedAt.Time
			}
			d := max(int64(end.Sub(s.StartedAt.Time).Seconds()), 0)
			sr.DurationSeconds = &d
		}
		if s.FinishedAt != nil {
			t := s.FinishedAt.Format(time.RFC3339)
			sr.FinishedAt = &t
		}
		r.Steps = append(r.Steps, sr)
	}
	return r
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
