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
import Skeleton from 'react-loading-skeleton';
import 'react-loading-skeleton/dist/skeleton.css';
import { useStyles2 } from '@grafana/ui';
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';

const getStyles = (theme: GrafanaTheme2) => ({
  skeleton: css`
    display: flex;
    padding: 12px 16px;
    gap: 12px;
  `,
  content: css`
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
  `,
  skeletonBase: css`
    --base-color: ${theme.colors.background.secondary};
    --highlight-color: ${theme.colors.background.canvas};
  `,
});

/**
 * Single message skeleton placeholder
 * Displays a loading animation while messages are being fetched
 */
export function MessageSkeleton() {
  const styles = useStyles2(getStyles);
  return (
    <div className={`${styles.skeleton} ${styles.skeletonBase}`}>
      <Skeleton circle height={32} width={32} />
      <div className={styles.content}>
        <Skeleton width="30%" height={14} />
        <Skeleton count={2} />
      </div>
    </div>
  );
}

/**
 * List of message skeleton placeholders
 * @param count - Number of skeleton placeholders to render (default: 5)
 */
export function MessageSkeletonList({ count = 5 }: { count?: number }) {
  const styles = useStyles2(getStyles);
  return (
    <div className={styles.skeletonBase}>
      {Array(count).fill(0).map((_, i) => (
        <MessageSkeleton key={i} />
      ))}
    </div>
  );
}
