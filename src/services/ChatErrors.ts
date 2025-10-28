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

export class AuthError extends Error {
  public readonly status?: number;
  public readonly code?: string;

  constructor(message: string, status?: number, code?: string) {
    super(message);
    this.name = 'AuthError';
    this.status = status;
    this.code = code;
  }
}

export class RateLimitError extends Error {
  public readonly status?: number;

  constructor(message: string, status?: number) {
    super(message);
    this.name = 'RateLimitError';
    this.status = status;
  }
}

export class NetworkError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'NetworkError';
  }
}

export class ServerError extends Error {
  public readonly status?: number;

  constructor(message: string, status?: number) {
    super(message);
    this.name = 'ServerError';
    this.status = status;
  }
}

export class NotFoundError extends Error {
  public readonly status?: number;
  public readonly code?: string;

  constructor(message: string, status?: number, code?: string) {
    super(message);
    this.name = 'NotFoundError';
    this.status = status;
    this.code = code;
  }
}

interface ErrorPayload {
  code?: string;
  message?: string;
  status?: number;
  error?: string;
}

function getErrorPayload(error: unknown): ErrorPayload {
  const maybeError = error as any;
  return maybeError?.data || {};
}

function getStatus(error: unknown): number | undefined {
  const maybeError = error as any;
  return maybeError?.status ?? maybeError?.statusCode ?? maybeError?.data?.status;
}

function getCode(error: unknown): string | undefined {
  const payload = getErrorPayload(error);
  return payload.code;
}

function getMessage(error: unknown): string {
  const payload = getErrorPayload(error);
  const fallback = error instanceof Error ? error.message : String(error);
  return payload.message || payload.error || fallback || 'Unknown error';
}

function isLikelyNetworkFailure(error: unknown): boolean {
  const maybeError = error as any;
  const message = (error instanceof Error ? error.message : String(error)).toLowerCase();
  return (
    maybeError?.name === 'TypeError' ||
    message.includes('network') ||
    message.includes('failed to fetch') ||
    message.includes('timeout') ||
    message.includes('aborted')
  );
}

export function classifyBackendError(error: unknown, operation: string): Error {
  const status = getStatus(error);
  const code = getCode(error);
  const message = getMessage(error);

  if (status === 404 || code === 'NOT_FOUND') {
    return new NotFoundError(message || `${operation}: not found`, status, code);
  }

  if (status === 401 || status === 403 || code === 'AUTH_REQUIRED') {
    return new AuthError(message || `${operation}: authentication required`, status, code);
  }

  if (status === 429) {
    return new RateLimitError(message || `${operation}: rate limit exceeded`, status);
  }

  if (typeof status === 'number' && status >= 500) {
    return new ServerError(message || `${operation}: backend error`, status);
  }

  if (!status || isLikelyNetworkFailure(error)) {
    return new NetworkError(message || `${operation}: network error`);
  }

  return new Error(`${operation}: ${message}`);
}

export function isAuthError(error: unknown): error is AuthError {
  if (error instanceof AuthError) {
    return true;
  }

  const maybeError = error as any;
  return maybeError?.code === 'AUTH_REQUIRED' || maybeError?.status === 401 || maybeError?.status === 403;
}

export function isNetworkError(error: unknown): error is NetworkError {
  if (error instanceof NetworkError) {
    return true;
  }

  const maybeError = error as any;
  return maybeError?.name === 'NetworkError';
}

export function isNotFoundError(error: unknown): error is NotFoundError {
  if (error instanceof NotFoundError) {
    return true;
  }

  const maybeError = error as any;
  return maybeError?.name === 'NotFoundError' || maybeError?.status === 404 || maybeError?.code === 'NOT_FOUND';
}
