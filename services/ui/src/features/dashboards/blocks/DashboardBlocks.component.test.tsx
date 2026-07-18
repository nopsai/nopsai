import { render, screen } from '@testing-library/react';
import { expect, test } from 'vitest';

import { DashboardBlocks } from './DashboardBlocks';
import { dashboardSpecNeedsWideLayout } from './DashboardBlocksLayout';

test('renders dashboard chart values from numeric strings with units', () => {
  const { container } = render(
    <DashboardBlocks
      spec={{
        title: 'Build Duration',
        blocks: [
          {
            type: 'chart',
            title: 'Build Duration by Image',
            chart: {
              type: 'bar',
              unit: 's',
              series: [
                {
                  key: 'duration',
                  label: 'Seconds',
                  points: [
                    { label: 'nopsai-dashboard', value: '24s' },
                    { label: 'git-sample', value: 55 },
                  ],
                },
              ],
            },
          },
        ],
      }}
    />
  );

  expect(screen.getByRole('img', { name: 'Dashboard bar chart' })).toBeVisible();
  expect(screen.getByText('55s')).toBeVisible();
  expect(screen.getByText('git-sample')).toBeVisible();
  expect(screen.queryByText('bar')).not.toBeInTheDocument();
  expect(screen.getByText('Slowest: 55s')).toBeVisible();
  expect(container.querySelectorAll('[style*="width"]')).toHaveLength(2);
});

test('renders donut charts as circular visualizations', () => {
  const { container } = render(
    <DashboardBlocks
      spec={{
        title: 'Readiness',
        blocks: [
          {
            type: 'chart',
            title: 'Production readiness',
            chart: {
              type: 'donut',
              series: [
                {
                  key: 'readiness',
                  points: [
                    { label: 'Ready', value: 1 },
                    { label: 'Blocked', value: 3 },
                  ],
                },
              ],
            },
          },
        ],
      }}
    />
  );

  expect(screen.getByRole('img', { name: 'donut dashboard chart' })).toBeVisible();
  expect(screen.getByText('Ready')).toBeVisible();
  expect(screen.getByText('Blocked')).toBeVisible();
  expect(screen.getByText('1/4')).toBeVisible();
  expect(screen.getByText('ready')).toBeVisible();
  expect(container.querySelector('.dashboard-circular-chart')).toBeTruthy();
  expect(container.querySelector('.dashboard-circular-chart__body')).toBeTruthy();
  expect(container.querySelector('.dashboard-circular-chart__legend-name')).toHaveTextContent('Ready');
  const slices = Array.from(container.querySelectorAll('svg circle[stroke-dasharray]'));
  expect(new Set(slices.map(slice => slice.getAttribute('stroke'))).size).toBe(2);
});

test('renders property blocks as metric cards with secondary text', () => {
  render(
    <DashboardBlocks
      spec={{
        title: 'Overview',
        blocks: [
          {
            type: 'properties',
            items: [
              { label: 'Total Build Time', value: '151s', text: 'Average 37.75s' },
            ],
          },
        ],
      }}
    />
  );

  expect(screen.getByText('Total Build Time')).toBeVisible();
  expect(screen.getByText('151s')).toBeVisible();
  expect(screen.getByText('Average 37.75s')).toBeVisible();
});

test('renders boolean-like table cells as status chips', () => {
  render(
    <DashboardBlocks
      spec={{
        title: 'Image comparison',
        blocks: [
          {
            type: 'table',
            columns: [
              { key: 'image', label: 'Image' },
              { key: 'vulnerabilities', label: 'Vulnerabilities' },
              { key: 'ready', label: 'Prod Ready' },
            ],
            rows: [
              { image: 'app-finance', vulnerabilities: 'Yes', ready: 'No' },
            ],
          },
        ],
      }}
    />
  );

  expect(screen.getByText('app-finance')).toBeVisible();
  expect(screen.getByText('Yes')).toHaveClass('text-rose-800');
  expect(screen.getByText('No')).toHaveClass('text-rose-800');
});

test('groups overview duration and donut charts into a wide operational layout', () => {
  const { container } = render(
    <DashboardBlocks
      spec={{
        title: 'Docker Image Operations Overview',
        blocks: [
          {
            type: 'properties',
            items: [
              { label: 'Images Built', value: '4', text: 'Pipeline completed' },
              { label: 'Total Build Time', value: '151s', text: 'Average 37.75s' },
              { label: 'Production Ready', value: '0 / 4', text: '4 images blocked' },
              { label: 'Configuration Present', value: '2 / 4', text: '2 images incomplete' },
            ],
          },
          {
            type: 'chart',
            title: 'Build Duration',
            chart: {
              type: 'bar',
              unit: 's',
              series: [
                {
                  key: 'build_duration',
                  label: 'Build Duration',
                  points: [
                    { label: 'nopsai-dashboard:latest', value: 24 },
                    { label: 'git-sample:dev', value: 55 },
                    { label: 'app-finance:prod', value: 60 },
                    { label: 'seed-static:3.4.5', value: 12 },
                  ],
                },
              ],
            },
          },
          {
            type: 'chart',
            title: 'Production Readiness',
            chart: {
              type: 'donut',
              series: [
                {
                  key: 'production_readiness',
                  points: [
                    { label: 'Production Ready', value: 0 },
                    { label: 'Blocked From Production', value: 4 },
                  ],
                },
              ],
            },
          },
          {
            type: 'chart',
            title: 'Runtime Configuration',
            chart: {
              type: 'donut',
              series: [
                {
                  key: 'runtime_configuration',
                  points: [
                    { label: 'Configuration Present', value: 2 },
                    { label: 'Missing Runtime Configuration', value: 2 },
                  ],
                },
              ],
            },
          },
        ],
      }}
    />
  );

  expect(screen.getByTestId('dashboard-overview-chart-grid')).toHaveClass('xl:grid-cols-[minmax(0,0.95fr)_minmax(0,1.05fr)]');
  expect(screen.getByTestId('dashboard-overview-chart-grid')).toHaveClass('items-stretch');
  expect(screen.getByText('Slowest: 60s')).toBeVisible();
  expect(screen.getByText('0/4')).toBeVisible();
  expect(screen.getByText('2/4')).toBeVisible();
  const productionReadinessSlices = Array.from(container.querySelectorAll('svg[aria-label="donut dashboard chart"] circle[stroke-dasharray]')).slice(0, 2);
  expect(productionReadinessSlices.map(slice => slice.getAttribute('stroke'))).toEqual(['#2563eb', '#94a3b8']);
});

test('marks rich dashboard specs as needing full-width publication cards', () => {
  expect(
    dashboardSpecNeedsWideLayout({
      title: 'Docker Image Operations Overview',
      blocks: [
        {
          type: 'properties',
          items: [
            { label: 'Images Built', value: '4' },
            { label: 'Total Build Time', value: '151s' },
            { label: 'Production Ready', value: '0 / 4' },
            { label: 'Configuration Present', value: '2 / 4' },
          ],
        },
      ],
    })
  ).toBe(true);

  expect(
    dashboardSpecNeedsWideLayout({
      title: 'Small status',
      blocks: [{ type: 'status', label: 'Ready', value: 'true' }],
    })
  ).toBe(false);
});
