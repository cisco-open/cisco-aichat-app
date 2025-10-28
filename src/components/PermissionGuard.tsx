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
import { css } from '@emotion/css';
import { GrafanaTheme2 } from '@grafana/data';
import { Alert, useStyles2 } from '@grafana/ui';
import { PermissionService } from '../services/PermissionService';

const getStyles = (theme: GrafanaTheme2) => ({
  container: css`
    height: 100vh;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: ${theme.spacing(4)};
  `,
  accessDenied: css`
    max-width: 600px;
    text-align: center;
  `,
  userInfo: css`
    margin-top: ${theme.spacing(2)};
    padding: ${theme.spacing(2)};
    background: ${theme.colors.background.secondary};
    border-radius: ${theme.shape.radius.default};
    font-size: ${theme.typography.bodySmall.fontSize};
    color: ${theme.colors.text.secondary};
  `
});

interface PermissionGuardProps {
  children: React.ReactNode;
  appName: string;
}

export function PermissionGuard({ children, appName }: PermissionGuardProps) {
  const s = useStyles2(getStyles);
  const permissionService = PermissionService.getInstance();
  const userInfo = permissionService.getUserInfo();

  // Check if user has permission to access the app
  if (!permissionService.canAccessApps()) {
    return (
      <div className={s.container}>
        <div className={s.accessDenied}>
          <Alert 
            severity="warning" 
            title={`Access Denied - ${appName}`}
          >
            <div>
              <p>
                You do not have permission to access this application.
                Access to {appName} is restricted to users with <strong>Admin</strong> or <strong>Editor</strong> roles.
              </p>
              <p>
                Please contact your Grafana administrator to request the appropriate permissions.
              </p>
              <div className={s.userInfo}>
                <strong>Current User:</strong> {userInfo.name} ({userInfo.login})<br />
                <strong>Role:</strong> {userInfo.role}<br />
                <strong>Required Roles:</strong> Admin or Editor
              </div>
            </div>
          </Alert>
        </div>
      </div>
    );
  }

  // User has permission, render the app content
  return <>{children}</>;
}
