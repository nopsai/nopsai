import { useCallback, useMemo, useRef, useState } from 'react';
import { apiClient } from '../../lib/api';
import { DEFAULT_PIPELINE_NAME } from '../../lib/lab';

export type LabOverride = {
  id: number;
  key: string;
  value: string;
};

export type LabFeedback = {
  tone: 'success' | 'error' | 'info';
  message: string;
  runId?: string;
} | null;

type LabSessionState = {
  version: 1;
  selectedPipelineId: string;
  yaml: string;
  originalYaml: string;
  scope: string;
  overrides: Array<{ key: string; value: string }>;
};

const LAB_SESSION_STORAGE_KEY = 'nopsai.lab.session.v1';

export function buildBlankLabYaml(name = DEFAULT_PIPELINE_NAME) {
  return [
    `name: ${name}`,
    'version: latest',
    'description: Lab pipeline (ad-hoc)',
    'container_image: alpine:latest',
    'steps:',
    '  - name: hello',
    '    script: echo "Hello from Lab"',
    '',
  ].join('\n');
}

function loadLabSession(): LabSessionState | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = sessionStorage.getItem(LAB_SESSION_STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<LabSessionState>;
    if (!parsed || parsed.version !== 1 || typeof parsed.yaml !== 'string') return null;
    return {
      version: 1,
      selectedPipelineId:
        typeof parsed.selectedPipelineId === 'string' ? parsed.selectedPipelineId : '',
      yaml: parsed.yaml,
      originalYaml:
        typeof parsed.originalYaml === 'string' ? parsed.originalYaml : parsed.yaml,
      scope: typeof parsed.scope === 'string' ? parsed.scope : '',
      overrides: Array.isArray(parsed.overrides)
        ? parsed.overrides.map(row => ({
            key: typeof row?.key === 'string' ? row.key : '',
            value: typeof row?.value === 'string' ? row.value : '',
          }))
        : [],
    };
  } catch {
    return null;
  }
}

function encodePipelineID(id: string) {
  return id.split('/').map(encodeURIComponent).join('/');
}

export function useLabSession() {
  const initialSession = useMemo(loadLabSession, []);
  const blankYaml = useMemo(() => buildBlankLabYaml(), []);
  const [selectedPipelineId, setSelectedPipelineId] = useState(
    initialSession?.selectedPipelineId ?? ''
  );
  const [yamlText, setYamlText] = useState(initialSession?.yaml ?? blankYaml);
  const [originalYaml, setOriginalYaml] = useState(
    initialSession?.originalYaml ?? initialSession?.yaml ?? blankYaml
  );
  const [scopeValue, setScopeValue] = useState(initialSession?.scope ?? '');
  const overrideIdRef = useRef(initialSession?.overrides.length ?? 0);
  const [overrides, setOverrides] = useState<LabOverride[]>(() =>
    (initialSession?.overrides ?? []).map((row, index) => ({
      id: index + 1,
      key: row.key,
      value: row.value,
    }))
  );
  const [feedback, setFeedback] = useState<LabFeedback>(null);
  const [yamlLoading, setYamlLoading] = useState(false);

  const hasUnsavedChanges = yamlText !== originalYaml;

  const saveSession = useCallback(
    (validationErrorCount: number) => {
      if (validationErrorCount > 0) {
        setFeedback({ tone: 'error', message: 'Fix validation errors before saving.' });
        return false;
      }

      setOriginalYaml(yamlText);
      try {
        const payload: LabSessionState = {
          version: 1,
          selectedPipelineId,
          yaml: yamlText,
          originalYaml: yamlText,
          scope: scopeValue,
          overrides: overrides.map(row => ({ key: row.key, value: row.value })),
        };
        sessionStorage.setItem(LAB_SESSION_STORAGE_KEY, JSON.stringify(payload));
      } catch (error) {
        console.warn('Unable to save Lab session state', error);
      }
      setFeedback({
        tone: 'success',
        message: 'YAML saved for this lab session (pipelines unchanged).',
      });
      return true;
    },
    [overrides, scopeValue, selectedPipelineId, yamlText]
  );

  const changePipeline = useCallback(
    async (nextId: string, options?: { reload?: boolean; resetOverrides?: boolean }) => {
      if (nextId === selectedPipelineId && !options?.reload) return true;
      if (hasUnsavedChanges && !window.confirm('Discard your current Lab edits?')) {
        return false;
      }

      setFeedback(null);
      setYamlLoading(true);
      setSelectedPipelineId(nextId);
      if (options?.resetOverrides) {
        setOverrides([]);
      }
      try {
        if (!nextId) {
          const nextBlankYaml = buildBlankLabYaml();
          setYamlText(nextBlankYaml);
          setOriginalYaml(nextBlankYaml);
          return true;
        }

        const response = await apiClient.fetch(`/v1/pipelines/${encodePipelineID(nextId)}`);
        if (!response.ok) {
          const text = await response.text();
          throw new Error(text || `Failed to load pipeline YAML (${response.status})`);
        }
        const rawYaml = await response.text();
        setYamlText(rawYaml);
        setOriginalYaml(rawYaml);
        return true;
      } catch (error) {
        console.error('Failed to load pipeline YAML', error);
        setFeedback({
          tone: 'error',
          message: error instanceof Error ? error.message : 'Unable to load pipeline YAML',
        });
        return false;
      } finally {
        setYamlLoading(false);
      }
    },
    [hasUnsavedChanges, selectedPipelineId]
  );

  const addOverride = useCallback(() => {
    overrideIdRef.current += 1;
    setOverrides(current => [
      ...current,
      { id: overrideIdRef.current, key: '', value: '' },
    ]);
  }, []);

  const updateOverride = useCallback(
    (id: number, field: 'key' | 'value', value: string) => {
      setOverrides(current =>
        current.map(row => (row.id === id ? { ...row, [field]: value } : row))
      );
    },
    []
  );

  const removeOverride = useCallback((id: number) => {
    setOverrides(current => current.filter(row => row.id !== id));
  }, []);

  return {
    addOverride,
    changePipeline,
    feedback,
    hasUnsavedChanges,
    overrides,
    removeOverride,
    saveSession,
    scopeValue,
    selectedPipelineId,
    setFeedback,
    setScopeValue,
    setYamlText,
    updateOverride,
    yamlLoading,
    yamlText,
  };
}
