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
import { useStyles2 } from '@grafana/ui';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { vscDarkPlus, vs } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { ChatMessage as ChatMessageType } from '../types/chat';
import { SummarizedMessage } from './SummarizedMessage';

interface ChatMessageProps {
  message: ChatMessageType;
}

/**
 * ChatMessage Component
 *
 * Displays a single chat message with markdown rendering and syntax highlighting.
 * Supports user and assistant messages with distinct styling.
 *
 * Features:
 * - Markdown rendering with GitHub Flavored Markdown (GFM)
 * - Syntax highlighting for code blocks
 * - Responsive design with max-width constraints
 * - Timestamp display
 * - Theme-aware styling
 */
export function ChatMessage({ message }: ChatMessageProps) {
  const s = useStyles2(getStyles);

  // Check if this is a summary message - render differently
  if (message.isSummary) {
    return (
      <SummarizedMessage
        summaryText={message.content}
        summarizedCount={message.summarizedIds?.length || 0}
        timestamp={message.timestamp}
        summaryDepth={message.summaryDepth}
      />
    );
  }

  const isUser = message.role === 'user';

  return (
    <div className={s.messageWrapper}>
      <div className={isUser ? s.userMessage : s.assistantMessage}>
        <div className={s.messageHeader}>
          <div className={s.headerLeft}>
            <strong>{isUser ? 'You' : 'AI Assistant'}</strong>
          </div>
          <div className={s.headerRight}>
            <span className={s.timestamp}>
              {new Date(message.timestamp).toLocaleTimeString()}
            </span>
          </div>
        </div>
        <div className={s.messageContent}>
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              // Custom code block renderer with syntax highlighting
              code(props: any) {
                const { node, className, children, ...rest } = props;
                const inline = !className;
                const match = /language-(\w+)/.exec(className || '');
                const language = match ? match[1] : '';

                return !inline && match ? (
                  <div className={s.codeBlock}>
                    <SyntaxHighlighter
                      style={s.isDarkTheme ? vscDarkPlus : (vs as any)}
                      language={language}
                      PreTag="div"
                      customStyle={{
                        margin: '0.5em 0',
                        borderRadius: '4px',
                        fontSize: '0.9em',
                      }}
                      {...rest}
                    >
                      {String(children).replace(/\n$/, '')}
                    </SyntaxHighlighter>
                  </div>
                ) : (
                  <code className={s.inlineCode} {...props}>
                    {children}
                  </code>
                );
              },
              // Custom paragraph renderer
              p({ children }) {
                return <p className={s.paragraph}>{children}</p>;
              },
              // Custom heading renderers
              h1({ children }) {
                return <h1 className={s.heading1}>{children}</h1>;
              },
              h2({ children }) {
                return <h2 className={s.heading2}>{children}</h2>;
              },
              h3({ children }) {
                return <h3 className={s.heading3}>{children}</h3>;
              },
              // Custom list renderers
              ul({ children }) {
                return <ul className={s.list}>{children}</ul>;
              },
              ol({ children }) {
                return <ol className={s.orderedList}>{children}</ol>;
              },
              li({ children }) {
                return <li className={s.listItem}>{children}</li>;
              },
              // Custom link renderer
              a({ href, children }) {
                return (
                  <a href={href} className={s.link} target="_blank" rel="noopener noreferrer">
                    {children}
                  </a>
                );
              },
              // Custom blockquote renderer
              blockquote({ children }) {
                return <blockquote className={s.blockquote}>{children}</blockquote>;
              },
              // Custom strong/bold renderer
              strong({ children }) {
                return <strong className={s.strong}>{children}</strong>;
              },
              // Custom emphasis/italic renderer
              em({ children }) {
                return <em className={s.emphasis}>{children}</em>;
              },
              // Custom table renderers
              table({ children }) {
                return <table className={s.table}>{children}</table>;
              },
              thead({ children }) {
                return <thead className={s.tableHead}>{children}</thead>;
              },
              tbody({ children }) {
                return <tbody className={s.tableBody}>{children}</tbody>;
              },
              tr({ children }) {
                return <tr className={s.tableRow}>{children}</tr>;
              },
              th({ children }) {
                return <th className={s.tableHeader}>{children}</th>;
              },
              td({ children }) {
                return <td className={s.tableCell}>{children}</td>;
              },
            }}
          >
            {message.content}
          </ReactMarkdown>
        </div>
      </div>
    </div>
  );
}

const getStyles = (theme: GrafanaTheme2) => {
  const isDarkTheme = theme.isDark;

  return {
    isDarkTheme,

    messageWrapper: css`
      display: flex;
      flex-direction: column;
    `,

    userMessage: css`
      align-self: flex-end;
      max-width: 70%;
      min-width: 0;
      background: ${theme.colors.primary.main};
      color: ${theme.colors.primary.contrastText};
      padding: ${theme.spacing(1.5)};
      border-radius: ${theme.shape.radius.default};
      border-bottom-right-radius: 4px;
      overflow-x: auto;
      overflow-y: hidden;
    `,

    assistantMessage: css`
      align-self: flex-start;
      max-width: 70%;
      min-width: 0;
      background: ${theme.colors.background.secondary};
      color: ${theme.colors.text.primary};
      padding: ${theme.spacing(1.5)};
      border-radius: ${theme.shape.radius.default};
      border-bottom-left-radius: 4px;
      border: 1px solid ${theme.colors.border.weak};
      overflow-x: auto;
      overflow-y: hidden;
    `,

    messageHeader: css`
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: ${theme.spacing(0.5)};
      font-size: ${theme.typography.bodySmall.fontSize};
    `,

    headerLeft: css`
      display: flex;
      align-items: center;
      gap: ${theme.spacing(0.5)};
    `,

    headerRight: css`
      display: flex;
      align-items: center;
      gap: ${theme.spacing(1)};
    `,

    timestamp: css`
      opacity: 0.7;
      font-size: ${theme.typography.bodySmall.fontSize};
    `,

    messageContent: css`
      line-height: 1.6;
      word-wrap: break-word;
      overflow-wrap: break-word;
      max-width: 100%;
      min-width: 0;

      /* Reset default margins for markdown elements */
      > *:first-child {
        margin-top: 0;
      }

      > *:last-child {
        margin-bottom: 0;
      }
    `,

    paragraph: css`
      margin: ${theme.spacing(1)} 0;
      line-height: 1.6;
    `,

    heading1: css`
      font-size: ${theme.typography.h2.fontSize};
      font-weight: ${theme.typography.fontWeightBold};
      margin: ${theme.spacing(2)} 0 ${theme.spacing(1)} 0;
      color: ${theme.colors.text.primary};
      border-bottom: 1px solid ${theme.colors.border.weak};
      padding-bottom: ${theme.spacing(0.5)};
    `,

    heading2: css`
      font-size: ${theme.typography.h3.fontSize};
      font-weight: ${theme.typography.fontWeightMedium};
      margin: ${theme.spacing(1.5)} 0 ${theme.spacing(1)} 0;
      color: ${theme.colors.text.primary};
    `,

    heading3: css`
      font-size: ${theme.typography.h4.fontSize};
      font-weight: ${theme.typography.fontWeightMedium};
      margin: ${theme.spacing(1.5)} 0 ${theme.spacing(0.5)} 0;
      color: ${theme.colors.text.primary};
    `,

    list: css`
      margin: ${theme.spacing(1)} 0;
      padding-left: ${theme.spacing(3)};
      line-height: 1.6;
    `,

    orderedList: css`
      margin: ${theme.spacing(1)} 0;
      padding-left: ${theme.spacing(3)};
      line-height: 1.6;
    `,

    listItem: css`
      margin: ${theme.spacing(0.5)} 0;
    `,

    link: css`
      color: ${theme.colors.primary.text};
      text-decoration: underline;
      cursor: pointer;

      &:hover {
        color: ${theme.colors.primary.main};
      }
    `,

    inlineCode: css`
      background: ${theme.colors.background.canvas};
      border: 1px solid ${theme.colors.border.weak};
      padding: 2px 6px;
      border-radius: 3px;
      font-family: ${theme.typography.fontFamilyMonospace};
      font-size: 0.9em;
      color: ${theme.colors.text.primary};
    `,

    codeBlock: css`
      margin: ${theme.spacing(1)} 0;
      border-radius: 4px;
      max-width: 100%;
      box-sizing: border-box;

      /* Override default syntax highlighter styles */
      pre {
        margin: 0 !important;
        padding: ${theme.spacing(1)} !important;
        background: transparent !important;
        max-width: 100% !important;
        overflow: visible !important;
        box-sizing: border-box !important;
      }

      /* Force code content to respect container */
      code {
        display: block !important;
        white-space: pre !important;
        box-sizing: border-box !important;
      }
    `,

    blockquote: css`
      border-left: 4px solid ${theme.colors.primary.main};
      padding-left: ${theme.spacing(2)};
      margin: ${theme.spacing(1)} 0;
      color: ${theme.colors.text.secondary};
      font-style: italic;
    `,

    strong: css`
      font-weight: ${theme.typography.fontWeightBold};
      color: ${theme.colors.text.primary};
    `,

    emphasis: css`
      font-style: italic;
    `,

    table: css`
      max-width: 100%;
      border-collapse: collapse;
      margin: ${theme.spacing(1)} 0;
      border: 1px solid ${theme.colors.border.weak};
      table-layout: fixed;
      word-break: break-word;
    `,

    tableHead: css`
      background: ${theme.colors.background.canvas};
    `,

    tableBody: css`
      /* Table body styles */
    `,

    tableRow: css`
      border-bottom: 1px solid ${theme.colors.border.weak};

      &:last-child {
        border-bottom: none;
      }
    `,

    tableHeader: css`
      padding: ${theme.spacing(1)};
      text-align: left;
      font-weight: ${theme.typography.fontWeightBold};
      border-right: 1px solid ${theme.colors.border.weak};

      &:last-child {
        border-right: none;
      }
    `,

    tableCell: css`
      padding: ${theme.spacing(1)};
      border-right: 1px solid ${theme.colors.border.weak};
      word-break: break-word;
      overflow-wrap: break-word;

      &:last-child {
        border-right: none;
      }
    `,
  };
};
