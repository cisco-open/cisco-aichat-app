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

import { of, throwError, Observable } from 'rxjs';

/**
 * Mock fetch function for @grafana/runtime's getBackendSrv().fetch()
 */
export const mockFetch = jest.fn();

/**
 * Mock backend service for @grafana/runtime
 */
export const mockBackendSrv = {
  fetch: mockFetch,
};

/**
 * Reset all mocks to their initial state
 */
export const resetMocks = (): void => {
  mockFetch.mockReset();
};

/**
 * Helper to create a successful fetch response Observable
 */
export function createMockResponse<T>(data: T, status = 200): Observable<{ status: number; data: T }> {
  return of({ status, data });
}

/**
 * Helper to create an error fetch response Observable
 */
export function createMockError(error: Error | string): Observable<never> {
  const err = typeof error === 'string' ? new Error(error) : error;
  return throwError(() => err);
}

/**
 * Mock localStorage helper for controlled tests.
 * Note: jsdom provides working localStorage, so real localStorage can be used in most tests.
 * This helper is useful for tests that need more control over localStorage behavior.
 */
export interface MockLocalStorage {
  getItem: jest.Mock<string | null, [string]>;
  setItem: jest.Mock<void, [string, string]>;
  removeItem: jest.Mock<void, [string]>;
  clear: jest.Mock<void, []>;
  store: Record<string, string>;
}

export const createMockLocalStorage = (): MockLocalStorage => {
  const store: Record<string, string> = {};
  return {
    getItem: jest.fn((key: string) => store[key] ?? null),
    setItem: jest.fn((key: string, value: string) => {
      store[key] = value;
    }),
    removeItem: jest.fn((key: string) => {
      delete store[key];
    }),
    clear: jest.fn(() => {
      Object.keys(store).forEach((k) => delete store[k]);
    }),
    store,
  };
};
