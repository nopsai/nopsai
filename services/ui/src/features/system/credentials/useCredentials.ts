import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import {
  activateCredentialVersion,
  createCredential,
  deleteCredential,
  deleteCredentialVersion,
  disableCredential,
  enableCredential,
  fetchCredential,
  fetchCredentials,
  rotateCredential,
} from './api';
import { emptyCredentialForm, type CredentialFormState, type CredentialRecord } from './model';

export function useCredentials({ canManage }: { canManage: boolean }) {
  const [credentials, setCredentials] = useState<CredentialRecord[]>([]);
  const [selected, setSelected] = useState<CredentialRecord | null>(null);
  const [form, setForm] = useState<CredentialFormState>(emptyCredentialForm);
  const [rotationValue, setRotationValue] = useState('');
  const [creating, setCreating] = useState(false);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const selectionRequestID = useRef(0);

  const upsertLocal = useCallback((record: CredentialRecord) => {
    setCredentials(current =>
      [...current.filter(item => item.id !== record.id), record].sort((left, right) =>
        left.reference.localeCompare(right.reference)
      )
    );
    setSelected(record);
  }, []);

  const loadCredentials = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const records = await fetchCredentials();
      setCredentials(records);
      setSelected(current => records.find(record => record.id === current?.id) ?? null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to load credentials');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      void loadCredentials();
    }, 0);
    return () => window.clearTimeout(timeout);
  }, [loadCredentials]);

  const selectCredential = useCallback(async (record: CredentialRecord) => {
    const requestID = selectionRequestID.current + 1;
    selectionRequestID.current = requestID;
    setCreating(false);
    setError(null);
    setSelected(record);
    try {
      const detail = await fetchCredential(record.id);
      if (selectionRequestID.current === requestID) setSelected(detail);
    } catch (err) {
      if (selectionRequestID.current === requestID) {
        setError(err instanceof Error ? err.message : 'Unable to load credential details');
      }
    }
  }, []);

  const startCreate = useCallback(() => {
    if (!canManage) return;
    selectionRequestID.current += 1;
    setSelected(null);
    setForm(emptyCredentialForm);
    setRotationValue('');
    setCreating(true);
    setError(null);
  }, [canManage]);

  const closeDetails = useCallback(() => {
    selectionRequestID.current += 1;
    setSelected(null);
    setRotationValue('');
    setError(null);
  }, []);

  const submitCreate = useCallback(async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage) return;
    if (!form.name.trim() || !form.kind) {
      setError('Name and kind are required.');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      const record = await createCredential(form);
      upsertLocal(await fetchCredential(record.id));
      setForm(emptyCredentialForm);
      setCreating(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to create credential');
    } finally {
      setSaving(false);
    }
  }, [canManage, form, upsertLocal]);

  const submitRotation = useCallback(async (event: FormEvent) => {
    event.preventDefault();
    if (!canManage || !selected) return;
    if (!rotationValue) {
      setError('A new credential value is required.');
      return;
    }
    setSaving(true);
    setError(null);
    try {
      upsertLocal(await rotateCredential(selected.id, rotationValue));
      setRotationValue('');
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to rotate credential');
    } finally {
      setSaving(false);
    }
  }, [canManage, rotationValue, selected, upsertLocal]);

  const activateVersion = useCallback(async (version: number) => {
    if (!canManage || !selected) return;
    setSaving(true);
    setError(null);
    try {
      upsertLocal(await activateCredentialVersion(selected.id, version));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to activate credential version');
    } finally {
      setSaving(false);
    }
  }, [canManage, selected, upsertLocal]);

  const disableSelected = useCallback(async () => {
    if (!canManage || !selected) return;
    setSaving(true);
    setError(null);
    try {
      upsertLocal(await disableCredential(selected.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to disable credential');
    } finally {
      setSaving(false);
    }
  }, [canManage, selected, upsertLocal]);

  const enableSelected = useCallback(async () => {
    if (!canManage || !selected) return;
    setSaving(true);
    setError(null);
    try {
      upsertLocal(await enableCredential(selected.id));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to enable credential');
    } finally {
      setSaving(false);
    }
  }, [canManage, selected, upsertLocal]);

  const deleteVersion = useCallback(async (version: number) => {
    if (!canManage || !selected) return;
    if (!window.confirm(`Delete version ${version} of ${selected.reference}? This cannot be undone.`)) return;
    setSaving(true);
    setError(null);
    try {
      upsertLocal(await deleteCredentialVersion(selected.id, version));
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete credential version');
    } finally {
      setSaving(false);
    }
  }, [canManage, selected, upsertLocal]);

  const deleteSelected = useCallback(async () => {
    if (!canManage || !selected) return;
    if (!window.confirm(`Delete ${selected.reference}? This cannot be undone.`)) return;
    setSaving(true);
    setError(null);
    try {
      await deleteCredential(selected.id);
      setCredentials(current => current.filter(record => record.id !== selected.id));
      selectionRequestID.current += 1;
      setSelected(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unable to delete credential');
    } finally {
      setSaving(false);
    }
  }, [canManage, selected]);

  return {
    credentials,
    selected,
    form,
    setForm,
    rotationValue,
    setRotationValue,
    creating,
    setCreating,
    loading,
    saving,
    error,
    loadCredentials,
    selectCredential,
    startCreate,
    closeDetails,
    submitCreate,
    submitRotation,
    activateVersion,
    disableSelected,
    enableSelected,
    deleteVersion,
    deleteSelected,
  };
}
