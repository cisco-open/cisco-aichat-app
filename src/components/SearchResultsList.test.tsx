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

import React from 'react';
import { render, screen } from '@testing-library/react';
import { SearchResultsList } from './SearchResultsList';
import { SearchResult } from '../types/chat';

describe('SearchResultsList', () => {
  const onResultClick = jest.fn();

  const baseResult: SearchResult = {
    sessionId: 'session-1',
    sessionName: 'Session',
    messageId: 'message-1',
    content: 'plain content',
    timestamp: Date.now() - 60_000,
    role: 'assistant',
  };

  beforeEach(() => {
    jest.clearAllMocks();
  });

  it('renders highlighted content using mark tokens only', () => {
    const result: SearchResult = {
      ...baseResult,
      content: 'hello <mark>world</mark>',
    };

    render(<SearchResultsList results={[result]} isLoading={false} onResultClick={onResultClick} />);

    expect(screen.getByText('world', { selector: 'mark' })).toBeInTheDocument();
    expect(screen.getByRole('button')).toHaveTextContent('hello world');
  });

  it('does not execute or render arbitrary HTML from snippets', () => {
    const malicious = '<img src=x onerror=alert(1)> start <mark>match</mark> end <script>alert(1)</script>';
    const result: SearchResult = {
      ...baseResult,
      content: malicious,
    };

    const { container } = render(
      <SearchResultsList results={[result]} isLoading={false} onResultClick={onResultClick} />
    );

    expect(container.querySelector('img')).not.toBeInTheDocument();
    expect(container.querySelector('script')).not.toBeInTheDocument();
    expect(screen.getByText('match', { selector: 'mark' })).toBeInTheDocument();
    expect(screen.getByRole('button')).toHaveTextContent('<img src=x onerror=alert(1)>');
    expect(screen.getByRole('button')).toHaveTextContent('<script>alert(1)</script>');
  });
});
