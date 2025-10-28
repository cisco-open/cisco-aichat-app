/**
 * Copyright 2025 Cisco Systems, Inc. and its affiliates
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 */

import React, { useState, useRef, useEffect, useCallback } from 'react';
import { css, cx } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Input, useStyles2 } from '@grafana/ui';

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    display: inline-flex;
    align-items: center;
    max-width: 100%;
  `,
  displayText: css`
    cursor: pointer;
    padding: ${theme.spacing(0.25)} ${theme.spacing(0.5)};
    border-radius: ${theme.shape.radius.default};
    border: 1px solid transparent;
    transition: border-color 0.2s, background-color 0.2s;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100%;

    &:hover {
      border-color: ${theme.colors.border.medium};
      background-color: ${theme.colors.background.secondary};
    }
  `,
  placeholder: css`
    color: ${theme.colors.text.secondary};
    font-style: italic;
  `,
  inputWrapper: css`
    width: 100%;
    min-width: 120px;
  `,
  input: css`
    width: 100%;
  `
});

interface InlineEditableTextProps {
  value: string;
  onSave: (newValue: string) => Promise<void>;
  className?: string;
  placeholder?: string;
}

export function InlineEditableText({
  value,
  onSave,
  className,
  placeholder = 'Click to edit'
}: InlineEditableTextProps) {
  const s = useStyles2(getStyles);
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState(value);
  const [isSaving, setIsSaving] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);

  // Sync editValue when value prop changes
  useEffect(() => {
    if (!isEditing) {
      setEditValue(value);
    }
  }, [value, isEditing]);

  // Focus input when entering edit mode
  useEffect(() => {
    if (isEditing && inputRef.current) {
      inputRef.current.focus();
      inputRef.current.select();
    }
  }, [isEditing]);

  const handleSave = useCallback(async () => {
    const trimmedValue = editValue.trim();

    // Return early if value unchanged
    if (trimmedValue === value) {
      setIsEditing(false);
      return;
    }

    // Don't save empty values
    if (!trimmedValue) {
      setEditValue(value);
      setIsEditing(false);
      return;
    }

    setIsSaving(true);
    try {
      await onSave(trimmedValue);
      setIsEditing(false);
    } catch (error) {
      console.error('[InlineEditableText] Save failed:', error);
      // Keep editing mode open on error
    } finally {
      setIsSaving(false);
    }
  }, [editValue, value, onSave]);

  const handleCancel = useCallback(() => {
    setEditValue(value);
    setIsEditing(false);
  }, [value]);

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleSave();
    } else if (e.key === 'Escape') {
      e.preventDefault();
      handleCancel();
    }
  }, [handleSave, handleCancel]);

  // Click outside detection
  useEffect(() => {
    if (!isEditing) {
      return;
    }

    const handleClickOutside = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        handleSave();
      }
    };

    // Use mousedown for immediate response
    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, [isEditing, handleSave]);

  const enterEditMode = useCallback(() => {
    if (!isSaving) {
      setIsEditing(true);
    }
  }, [isSaving]);

  if (isEditing) {
    return (
      <div ref={containerRef} className={cx(s.container, s.inputWrapper, className)}>
        <Input
          ref={inputRef}
          className={s.input}
          value={editValue}
          onChange={(e) => setEditValue(e.currentTarget.value)}
          onKeyDown={handleKeyDown}
          disabled={isSaving}
          placeholder={placeholder}
        />
      </div>
    );
  }

  return (
    <div ref={containerRef} className={cx(s.container, className)}>
      <span
        className={cx(s.displayText, !value && s.placeholder)}
        onClick={enterEditMode}
        role="button"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            enterEditMode();
          }
        }}
        aria-label={value ? `Edit ${value}` : placeholder}
      >
        {value || placeholder}
      </span>
    </div>
  );
}
