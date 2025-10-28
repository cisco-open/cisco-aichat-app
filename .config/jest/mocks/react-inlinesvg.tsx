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

// Due to the grafana/ui Icon component making fetch requests to
// `/public/img/icon/<icon_name>.svg` we need to mock react-inlinesvg to prevent
// the failed fetch requests from displaying errors in console.

import React from 'react';

type Callback = (...args: any[]) => void;

export interface StorageItem {
  content: string;
  queue: Callback[];
  status: string;
}

export const cacheStore: { [key: string]: StorageItem } = Object.create(null);

const SVG_FILE_NAME_REGEX = /(.+)\/(.+)\.svg$/;

const InlineSVG = ({ src }: { src: string }) => {
  // testId will be the file name without extension (e.g. `public/img/icons/angle-double-down.svg` -> `angle-double-down`)
  const testId = src.replace(SVG_FILE_NAME_REGEX, '$2');
  return <svg xmlns="http://www.w3.org/2000/svg" data-testid={testId} viewBox="0 0 24 24" />;
};

export default InlineSVG;
