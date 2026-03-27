import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import yaml from 'js-yaml'
import YamlEditor from '../components/YamlEditor'

interface SecretRef {
  name: string
  key: string
}

interface RepoForm {
  owner: string
  name: string
  forkOwner: string
  description: string
  secretRef: SecretRef
}

interface StepForm {
  name: string
  type: string
  branchTemplate: string
  promptTemplate: string
  commands: string
  image: string
  onFailure: string
  maxRetries: number
  confidenceThreshold: string
}

const emptySecret = (): SecretRef => ({ name: '', key: 'token' })

const emptyRepo = (): RepoForm => ({
  owner: '', name: '', forkOwner: '', description: '', secretRef: emptySecret(),
})

const emptyStep = (): StepForm => ({
  name: '', type: 'ai', branchTemplate: '', promptTemplate: '',
  commands: '', image: '', onFailure: '', maxRetries: 0, confidenceThreshold: '0.7',
})

const stepTypes = ['git-checkout', 'ai', 'shell', 'git-push', 'triage'] as const

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
      <div className="px-5 py-3 border-b border-gray-800 bg-gray-900/80">
        <h3 className="text-sm font-medium text-gray-300 uppercase tracking-wider">{title}</h3>
      </div>
      <div className="p-5 space-y-4">{children}</div>
    </div>
  )
}

function Field({ label, children, hint }: { label: string; children: React.ReactNode; hint?: string }) {
  return (
    <label className="block">
      <span className="text-sm text-gray-400">{label}</span>
      {children}
      {hint && <span className="text-xs text-gray-600 mt-1 block">{hint}</span>}
    </label>
  )
}

const inputCls = 'mt-1 block w-full bg-gray-800 border border-gray-700 rounded px-3 py-2 text-sm text-gray-200 placeholder-gray-600 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500'
const textareaCls = inputCls + ' font-mono text-xs leading-relaxed'
const selectCls = inputCls + ' appearance-none'
const btnPrimary = 'px-4 py-2 rounded bg-indigo-600 text-white text-sm font-medium hover:bg-indigo-500 transition-colors disabled:opacity-50'
const btnSecondary = 'px-3 py-1.5 rounded bg-gray-800 text-gray-300 text-sm hover:bg-gray-700 border border-gray-700 transition-colors'
const btnDanger = 'px-2 py-1 rounded text-red-400 hover:bg-red-500/10 text-xs transition-colors'

function SecretRefFields({ value, onChange }: { value: SecretRef; onChange: (v: SecretRef) => void }) {
  return (
    <div className="grid grid-cols-2 gap-3">
      <Field label="Secret name">
        <input className={inputCls} value={value.name} placeholder="github-token"
          onChange={e => onChange({ ...value, name: e.target.value })} />
      </Field>
      <Field label="Secret key">
        <input className={inputCls} value={value.key} placeholder="token"
          onChange={e => onChange({ ...value, key: e.target.value })} />
      </Field>
    </div>
  )
}

export default function PipelineCreate() {
  const navigate = useNavigate()
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  // General
  const [name, setName] = useState('')
  const [namespace, setNamespace] = useState('default')

  // Trigger
  const [triggerType, setTriggerType] = useState<'github' | 'jira'>('github')
  const [ghOwner, setGhOwner] = useState('')
  const [ghRepo, setGhRepo] = useState('')
  const [ghAssignee, setGhAssignee] = useState('')
  const [ghPollInterval, setGhPollInterval] = useState('30s')
  const [ghLabels, setGhLabels] = useState('')
  const [ghSecret, setGhSecret] = useState<SecretRef>(emptySecret())
  const [jiraUrl, setJiraUrl] = useState('')
  const [jiraJql, setJiraJql] = useState('')
  const [jiraPollInterval, setJiraPollInterval] = useState('60s')
  const [jiraSecret, setJiraSecret] = useState<SecretRef>(emptySecret())

  // Repository
  const [repoMode, setRepoMode] = useState<'single' | 'multi'>('single')
  const [singleRepo, setSingleRepo] = useState<RepoForm>(emptyRepo())
  const [multiRepos, setMultiRepos] = useState<RepoForm[]>([emptyRepo()])

  // AI
  const [aiImage, setAiImage] = useState('')
  const [aiPullPolicy, setAiPullPolicy] = useState('IfNotPresent')
  const [aiSecret, setAiSecret] = useState<SecretRef>({ name: '', key: 'credentials.json' })
  const [aiMountPath, setAiMountPath] = useState('/tmp/gcp-creds.json')
  const [envVars, setEnvVars] = useState<{ key: string; value: string }[]>([])

  // Steps
  const [steps, setSteps] = useState<StepForm[]>([])

  // Mode
  const [mode, setMode] = useState<'form' | 'yaml'>('form')
  const [yamlContent, setYamlContent] = useState('')

  const switchToYaml = () => {
    const cr = {
      apiVersion: 'ai.aipipelines.io/v1alpha1',
      kind: 'Pipeline',
      metadata: { name: name || 'my-pipeline', namespace },
      spec: buildSpec(),
    }
    setYamlContent(yaml.dump(cr, { lineWidth: -1, noRefs: true, quotingType: '"' }))
    setMode('yaml')
  }

  const handleYamlSubmit = async () => {
    setSubmitting(true)
    setError('')
    try {
      const parsed = yaml.load(yamlContent) as Record<string, unknown>
      if (!parsed || typeof parsed !== 'object') throw new Error('Invalid YAML')

      const meta = parsed.metadata as Record<string, string> | undefined
      const crName = meta?.name || ''
      const crNamespace = meta?.namespace || 'default'
      if (!crName) throw new Error('metadata.name is required')

      const res = await fetch('/api/pipelines', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: crName, namespace: crNamespace, spec: parsed.spec }),
      })
      if (!res.ok) throw new Error(await res.text())
      navigate(`/pipelines/${crNamespace}/${crName}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create pipeline')
    } finally {
      setSubmitting(false)
    }
  }

  const updateStep = (idx: number, patch: Partial<StepForm>) => {
    setSteps(prev => prev.map((s, i) => i === idx ? { ...s, ...patch } : s))
  }
  const removeStep = (idx: number) => setSteps(prev => prev.filter((_, i) => i !== idx))
  const moveStep = (idx: number, dir: -1 | 1) => {
    setSteps(prev => {
      const next = [...prev]
      const target = idx + dir
      if (target < 0 || target >= next.length) return prev
      ;[next[idx], next[target]] = [next[target], next[idx]]
      return next
    })
  }

  const updateMultiRepo = (idx: number, patch: Partial<RepoForm>) => {
    setMultiRepos(prev => prev.map((r, i) => i === idx ? { ...r, ...patch } : r))
  }
  const removeMultiRepo = (idx: number) => setMultiRepos(prev => prev.filter((_, i) => i !== idx))

  const updateEnvVar = (idx: number, field: 'key' | 'value', val: string) => {
    setEnvVars(prev => prev.map((e, i) => i === idx ? { ...e, [field]: val } : e))
  }

  const buildSpec = () => {
    const spec: Record<string, unknown> = {}

    // Trigger
    if (triggerType === 'github') {
      const gh: Record<string, unknown> = {
        owner: ghOwner, repo: ghRepo, assignee: ghAssignee,
        pollInterval: ghPollInterval,
        secretRef: { name: ghSecret.name, key: ghSecret.key || 'token' },
      }
      const labels = ghLabels.split(',').map(l => l.trim()).filter(Boolean)
      if (labels.length) gh.labels = labels
      spec.trigger = { github: gh }
    } else {
      spec.trigger = {
        jira: {
          url: jiraUrl, jql: jiraJql, pollInterval: jiraPollInterval,
          secretRef: { name: jiraSecret.name, key: jiraSecret.key || 'token' },
        },
      }
    }

    // Repo
    if (repoMode === 'single') {
      spec.repo = {
        owner: singleRepo.owner, name: singleRepo.name,
        ...(singleRepo.forkOwner && { forkOwner: singleRepo.forkOwner }),
        secretRef: { name: singleRepo.secretRef.name, key: singleRepo.secretRef.key || 'token' },
      }
    } else {
      spec.repos = multiRepos.map(r => ({
        owner: r.owner, name: r.name,
        ...(r.description && { description: r.description }),
        ...(r.forkOwner && { forkOwner: r.forkOwner }),
        ...(r.secretRef.name && { secretRef: { name: r.secretRef.name, key: r.secretRef.key || 'token' } }),
      }))
    }

    // AI
    const ai: Record<string, unknown> = {
      image: aiImage,
      imagePullPolicy: aiPullPolicy,
    }
    if (aiSecret.name) {
      ai.secretRef = { name: aiSecret.name, key: aiSecret.key || 'credentials.json' }
      ai.credentialsMountPath = aiMountPath
    }
    const envMap: Record<string, string> = {}
    envVars.forEach(e => { if (e.key) envMap[e.key] = e.value })
    if (Object.keys(envMap).length) ai.env = envMap
    spec.ai = ai

    // Steps
    spec.steps = steps.map(s => {
      const step: Record<string, unknown> = { name: s.name, type: s.type }
      if (s.type === 'git-checkout' && s.branchTemplate) step.branchTemplate = s.branchTemplate
      if ((s.type === 'ai' || s.type === 'triage') && s.promptTemplate) step.promptTemplate = s.promptTemplate
      if (s.type === 'triage' && s.confidenceThreshold) step.confidenceThreshold = s.confidenceThreshold
      if (s.type === 'shell') {
        const cmds = s.commands.split('\n').filter(Boolean)
        if (cmds.length) step.commands = cmds
        if (s.image) step.image = s.image
      }
      if (s.onFailure) step.onFailure = s.onFailure
      if (s.maxRetries > 0) step.maxRetries = s.maxRetries
      return step
    })

    return spec
  }

  const handleSubmit = async () => {
    if (!name.trim()) { setError('Pipeline name is required'); return }
    if (!steps.length) { setError('At least one step is required'); return }
    if (steps.some(s => !s.name || !s.type)) { setError('All steps must have a name and type'); return }

    setSubmitting(true)
    setError('')
    try {
      const res = await fetch('/api/pipelines', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim(), namespace, spec: buildSpec() }),
      })
      if (!res.ok) throw new Error(await res.text())
      navigate(`/pipelines/${namespace}/${name.trim()}`)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create pipeline')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <div className="flex items-center gap-3">
          <button onClick={() => navigate('/')} className="text-gray-400 hover:text-white transition-colors">&larr;</button>
          <h1 className="text-2xl font-semibold">Create Pipeline</h1>
        </div>
        <div className="flex bg-gray-800 rounded-lg p-0.5">
          <button onClick={() => setMode('form')}
            className={`px-4 py-1.5 rounded-md text-sm font-medium transition-colors ${mode === 'form' ? 'bg-gray-700 text-white' : 'text-gray-400 hover:text-white'}`}>
            Form
          </button>
          <button onClick={() => switchToYaml()}
            className={`px-4 py-1.5 rounded-md text-sm font-medium transition-colors ${mode === 'yaml' ? 'bg-gray-700 text-white' : 'text-gray-400 hover:text-white'}`}>
            YAML
          </button>
        </div>
      </div>

      {mode === 'yaml' ? (
        <div className="space-y-6 max-w-4xl">
          <div className="bg-gray-900 border border-gray-800 rounded-lg overflow-hidden">
            <YamlEditor value={yamlContent} onChange={setYamlContent} />
          </div>
          {error && (
            <div className="bg-red-500/10 border border-red-500/30 rounded-lg px-4 py-3 text-red-400 text-sm">
              {error}
            </div>
          )}
          <div className="flex justify-end gap-3">
            <button onClick={() => navigate('/')} className={btnSecondary}>Cancel</button>
            <button onClick={handleYamlSubmit} disabled={submitting} className={btnPrimary}>
              {submitting ? 'Creating...' : 'Create Pipeline'}
            </button>
          </div>
        </div>
      ) : (
      <div className="space-y-6 max-w-4xl">
        {/* General */}
        <Section title="General">
          <div className="grid grid-cols-2 gap-4">
            <Field label="Pipeline name">
              <input className={inputCls} value={name} placeholder="my-pipeline"
                onChange={e => setName(e.target.value)} />
            </Field>
            <Field label="Namespace">
              <input className={inputCls} value={namespace}
                onChange={e => setNamespace(e.target.value)} />
            </Field>
          </div>
        </Section>

        {/* Trigger */}
        <Section title="Trigger">
          <div className="flex gap-2 mb-4">
            {(['github', 'jira'] as const).map(t => (
              <button key={t} onClick={() => setTriggerType(t)}
                className={`px-4 py-1.5 rounded text-sm font-medium transition-colors ${triggerType === t ? 'bg-indigo-600 text-white' : 'bg-gray-800 text-gray-400 hover:text-white'}`}>
                {t === 'github' ? 'GitHub' : 'Jira'}
              </button>
            ))}
          </div>

          {triggerType === 'github' ? (
            <div className="space-y-4">
              <div className="grid grid-cols-3 gap-3">
                <Field label="Owner">
                  <input className={inputCls} value={ghOwner} placeholder="org-name"
                    onChange={e => setGhOwner(e.target.value)} />
                </Field>
                <Field label="Repo">
                  <input className={inputCls} value={ghRepo} placeholder="repo-name"
                    onChange={e => setGhRepo(e.target.value)} />
                </Field>
                <Field label="Assignee">
                  <input className={inputCls} value={ghAssignee} placeholder="username"
                    onChange={e => setGhAssignee(e.target.value)} />
                </Field>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Poll interval">
                  <input className={inputCls} value={ghPollInterval}
                    onChange={e => setGhPollInterval(e.target.value)} />
                </Field>
                <Field label="Labels" hint="Comma-separated">
                  <input className={inputCls} value={ghLabels} placeholder="ai, bug"
                    onChange={e => setGhLabels(e.target.value)} />
                </Field>
              </div>
              <SecretRefFields value={ghSecret} onChange={setGhSecret} />
            </div>
          ) : (
            <div className="space-y-4">
              <Field label="Jira URL">
                <input className={inputCls} value={jiraUrl} placeholder="https://mycompany.atlassian.net"
                  onChange={e => setJiraUrl(e.target.value)} />
              </Field>
              <Field label="JQL query">
                <textarea className={textareaCls} rows={2} value={jiraJql}
                  placeholder='project = ENG AND assignee = currentUser() AND status = "To Do"'
                  onChange={e => setJiraJql(e.target.value)} />
              </Field>
              <div className="grid grid-cols-2 gap-3">
                <Field label="Poll interval">
                  <input className={inputCls} value={jiraPollInterval}
                    onChange={e => setJiraPollInterval(e.target.value)} />
                </Field>
                <div />
              </div>
              <SecretRefFields value={jiraSecret} onChange={setJiraSecret} />
            </div>
          )}
        </Section>

        {/* Repository */}
        <Section title="Repository">
          <div className="flex gap-2 mb-4">
            {([['single', 'Single Repo'], ['multi', 'Multiple (Triage)']] as const).map(([val, label]) => (
              <button key={val} onClick={() => setRepoMode(val as 'single' | 'multi')}
                className={`px-4 py-1.5 rounded text-sm font-medium transition-colors ${repoMode === val ? 'bg-indigo-600 text-white' : 'bg-gray-800 text-gray-400 hover:text-white'}`}>
                {label}
              </button>
            ))}
          </div>

          {repoMode === 'single' ? (
            <div className="space-y-4">
              <div className="grid grid-cols-3 gap-3">
                <Field label="Owner">
                  <input className={inputCls} value={singleRepo.owner} placeholder="org-name"
                    onChange={e => setSingleRepo(r => ({ ...r, owner: e.target.value }))} />
                </Field>
                <Field label="Name">
                  <input className={inputCls} value={singleRepo.name} placeholder="repo-name"
                    onChange={e => setSingleRepo(r => ({ ...r, name: e.target.value }))} />
                </Field>
                <Field label="Fork owner" hint="Optional — defaults to owner">
                  <input className={inputCls} value={singleRepo.forkOwner}
                    onChange={e => setSingleRepo(r => ({ ...r, forkOwner: e.target.value }))} />
                </Field>
              </div>
              <SecretRefFields value={singleRepo.secretRef}
                onChange={v => setSingleRepo(r => ({ ...r, secretRef: v }))} />
            </div>
          ) : (
            <div className="space-y-3">
              {multiRepos.map((repo, i) => (
                <div key={i} className="bg-gray-800/50 rounded-lg p-4 space-y-3 relative">
                  <button onClick={() => removeMultiRepo(i)} className={`absolute top-3 right-3 ${btnDanger}`}>Remove</button>
                  <div className="grid grid-cols-3 gap-3">
                    <Field label="Owner">
                      <input className={inputCls} value={repo.owner} placeholder="org-name"
                        onChange={e => updateMultiRepo(i, { owner: e.target.value })} />
                    </Field>
                    <Field label="Name">
                      <input className={inputCls} value={repo.name} placeholder="repo-name"
                        onChange={e => updateMultiRepo(i, { name: e.target.value })} />
                    </Field>
                    <Field label="Fork owner" hint="Optional">
                      <input className={inputCls} value={repo.forkOwner}
                        onChange={e => updateMultiRepo(i, { forkOwner: e.target.value })} />
                    </Field>
                  </div>
                  <Field label="Description" hint="Helps the AI pick the right repo during triage">
                    <input className={inputCls} value={repo.description} placeholder="What this repo is for"
                      onChange={e => updateMultiRepo(i, { description: e.target.value })} />
                  </Field>
                  <SecretRefFields value={repo.secretRef}
                    onChange={v => updateMultiRepo(i, { secretRef: v })} />
                </div>
              ))}
              <button onClick={() => setMultiRepos(prev => [...prev, emptyRepo()])} className={btnSecondary}>
                + Add repository
              </button>
            </div>
          )}
        </Section>

        {/* AI Runtime */}
        <Section title="AI Runtime">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Container image">
              <input className={inputCls} value={aiImage} placeholder="ai-pipelines-claude:latest"
                onChange={e => setAiImage(e.target.value)} />
            </Field>
            <Field label="Pull policy">
              <select className={selectCls} value={aiPullPolicy} onChange={e => setAiPullPolicy(e.target.value)}>
                <option value="IfNotPresent">IfNotPresent</option>
                <option value="Always">Always</option>
                <option value="Never">Never</option>
              </select>
            </Field>
          </div>
          <div className="grid grid-cols-3 gap-3">
            <Field label="Credentials secret" hint="Optional">
              <input className={inputCls} value={aiSecret.name} placeholder="ai-credentials"
                onChange={e => setAiSecret(s => ({ ...s, name: e.target.value }))} />
            </Field>
            <Field label="Secret key">
              <input className={inputCls} value={aiSecret.key} placeholder="credentials.json"
                onChange={e => setAiSecret(s => ({ ...s, key: e.target.value }))} />
            </Field>
            <Field label="Mount path">
              <input className={inputCls} value={aiMountPath}
                onChange={e => setAiMountPath(e.target.value)} />
            </Field>
          </div>
          <div>
            <span className="text-sm text-gray-400 block mb-2">Environment variables</span>
            <div className="space-y-2">
              {envVars.map((ev, i) => (
                <div key={i} className="flex gap-2 items-center">
                  <input className={inputCls + ' flex-1'} value={ev.key} placeholder="KEY"
                    onChange={e => updateEnvVar(i, 'key', e.target.value)} />
                  <span className="text-gray-600">=</span>
                  <input className={inputCls + ' flex-1'} value={ev.value} placeholder="value"
                    onChange={e => updateEnvVar(i, 'value', e.target.value)} />
                  <button onClick={() => setEnvVars(prev => prev.filter((_, j) => j !== i))} className={btnDanger}>
                    &times;
                  </button>
                </div>
              ))}
              <button onClick={() => setEnvVars(prev => [...prev, { key: '', value: '' }])} className={btnSecondary}>
                + Add variable
              </button>
            </div>
          </div>
        </Section>

        {/* Steps */}
        <Section title="Steps">
          <div className="space-y-3">
            {steps.map((step, i) => (
              <div key={i} className="bg-gray-800/50 rounded-lg p-4 space-y-3">
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-gray-600 w-5">{i + 1}.</span>
                    <input className={inputCls + ' w-40'} value={step.name} placeholder="step-name"
                      onChange={e => updateStep(i, { name: e.target.value })} />
                    <select className={selectCls + ' w-36'} value={step.type}
                      onChange={e => updateStep(i, { type: e.target.value })}>
                      {stepTypes.map(t => <option key={t} value={t}>{t}</option>)}
                    </select>
                  </div>
                  <div className="flex items-center gap-1">
                    <button onClick={() => moveStep(i, -1)} disabled={i === 0}
                      className="px-2 py-1 text-gray-500 hover:text-white disabled:opacity-20 text-sm">&uarr;</button>
                    <button onClick={() => moveStep(i, 1)} disabled={i === steps.length - 1}
                      className="px-2 py-1 text-gray-500 hover:text-white disabled:opacity-20 text-sm">&darr;</button>
                    <button onClick={() => removeStep(i)} className={btnDanger}>&times;</button>
                  </div>
                </div>

                {/* Type-specific fields */}
                {step.type === 'git-checkout' && (
                  <Field label="Branch template" hint="Go template — e.g. ai/{{.IssueNumber}}-{{.Timestamp}}">
                    <input className={inputCls} value={step.branchTemplate}
                      placeholder="ai/{{.IssueNumber}}-{{.Timestamp}}"
                      onChange={e => updateStep(i, { branchTemplate: e.target.value })} />
                  </Field>
                )}

                {(step.type === 'ai' || step.type === 'triage') && (
                  <Field label="Prompt template" hint="Go template — available: {{.IssueNumber}} {{.IssueKey}} {{.IssueTitle}} {{.IssueBody}} {{.Branch}} {{range .RepoCandidates}}...{{end}}">
                    <textarea className={textareaCls} rows={6} value={step.promptTemplate}
                      onChange={e => updateStep(i, { promptTemplate: e.target.value })} />
                  </Field>
                )}

                {step.type === 'triage' && (
                  <Field label="Confidence threshold" hint="0.0–1.0 — below this, pipeline waits for user input">
                    <input className={inputCls + ' w-24'} value={step.confidenceThreshold}
                      onChange={e => updateStep(i, { confidenceThreshold: e.target.value })} />
                  </Field>
                )}

                {step.type === 'shell' && (
                  <>
                    <Field label="Commands" hint="One command per line">
                      <textarea className={textareaCls} rows={3} value={step.commands}
                        placeholder="bash check.sh"
                        onChange={e => updateStep(i, { commands: e.target.value })} />
                    </Field>
                    <Field label="Image" hint="Optional — defaults to ubuntu:24.04">
                      <input className={inputCls} value={step.image} placeholder="ubuntu:24.04"
                        onChange={e => updateStep(i, { image: e.target.value })} />
                    </Field>
                  </>
                )}

                {/* Retry/failure fields — shown for all types except git-checkout and git-push */}
                {!['git-checkout', 'git-push'].includes(step.type) && (
                  <div className="grid grid-cols-2 gap-3 pt-2 border-t border-gray-700/50">
                    <Field label="On failure — jump to step" hint="Optional — step name to retry from">
                      <select className={selectCls} value={step.onFailure}
                        onChange={e => updateStep(i, { onFailure: e.target.value })}>
                        <option value="">None</option>
                        {steps.filter((_, j) => j !== i).map(s => (
                          <option key={s.name} value={s.name}>{s.name}</option>
                        ))}
                      </select>
                    </Field>
                    <Field label="Max retries">
                      <input type="number" className={inputCls + ' w-24'} min={0} value={step.maxRetries}
                        onChange={e => updateStep(i, { maxRetries: parseInt(e.target.value) || 0 })} />
                    </Field>
                  </div>
                )}
              </div>
            ))}

            <button onClick={() => setSteps(prev => [...prev, emptyStep()])} className={btnSecondary}>
              + Add step
            </button>
          </div>
        </Section>

        {/* Submit */}
        {error && (
          <div className="bg-red-500/10 border border-red-500/30 rounded-lg px-4 py-3 text-red-400 text-sm">
            {error}
          </div>
        )}
        <div className="flex justify-end gap-3">
          <button onClick={() => navigate('/')} className={btnSecondary}>Cancel</button>
          <button onClick={handleSubmit} disabled={submitting} className={btnPrimary}>
            {submitting ? 'Creating...' : 'Create Pipeline'}
          </button>
        </div>
      </div>
      )}
    </div>
  )
}
