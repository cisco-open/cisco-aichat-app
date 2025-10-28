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

import { config } from '@grafana/runtime';

export type GrafanaRole = 'Admin' | 'Editor' | 'Viewer';

export interface UserPermissions {
  isAdmin: boolean;
  isEditor: boolean;
  isViewer: boolean;
  role: GrafanaRole;
  canAccessApps: boolean;
}

export class PermissionService {
  private static instance: PermissionService;

  private constructor() {}

  public static getInstance(): PermissionService {
    if (!PermissionService.instance) {
      PermissionService.instance = new PermissionService();
    }
    return PermissionService.instance;
  }

  /**
   * Get current user information from Grafana
   */
  private getCurrentUser() {
    return config.bootData.user;
  }

  /**
   * Get user permissions based on Grafana role
   */
  public getUserPermissions(): UserPermissions {
    const user = this.getCurrentUser();

    // Default to most restrictive if user info is not available
    if (!user) {
      return {
        isAdmin: false,
        isEditor: false,
        isViewer: true,
        role: 'Viewer',
        canAccessApps: false
      };
    }

    // Check user role from Grafana
    const isAdmin = user.orgRole === 'Admin';
    const isEditor = user.orgRole === 'Editor';
    const isViewer = user.orgRole === 'Viewer';

    // Define who can access the apps (Admin and Editor only)
    const canAccessApps = isAdmin || isEditor;

    return {
      isAdmin,
      isEditor,
      isViewer,
      role: user.orgRole as GrafanaRole,
      canAccessApps
    };
  }

  /**
   * Check if current user can access AI/MCP apps
   */
  public canAccessApps(): boolean {
    const permissions = this.getUserPermissions();
    return permissions.canAccessApps;
  }

  /**
   * Check if current user is Admin
   */
  public isAdmin(): boolean {
    const permissions = this.getUserPermissions();
    return permissions.isAdmin;
  }

  /**
   * Check if current user is Editor or higher
   */
  public isEditorOrHigher(): boolean {
    const permissions = this.getUserPermissions();
    return permissions.isAdmin || permissions.isEditor;
  }

  /**
   * Get user role string for display
   */
  public getUserRole(): string {
    const permissions = this.getUserPermissions();
    return permissions.role;
  }

  /**
   * Get user info for display
   */
  public getUserInfo() {
    const user = this.getCurrentUser();
    const permissions = this.getUserPermissions();

    return {
      name: user?.name || 'Unknown User',
      login: user?.login || 'unknown',
      email: user?.email || '',
      role: permissions.role,
      canAccessApps: permissions.canAccessApps
    };
  }
}
