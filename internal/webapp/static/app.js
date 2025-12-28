import React, { useEffect, useMemo, useRef, useState } from 'https://esm.sh/react@18.3.1';
import { createRoot } from 'https://esm.sh/react-dom@18.3.1/client';
import { Terminal } from 'https://esm.sh/@xterm/xterm@5.5.0';
import { FitAddon } from 'https://esm.sh/@xterm/addon-fit@0.10.0';
import jsyaml from 'https://esm.sh/js-yaml@4.1.0';

const overviewItem = { id: 'overview', title: 'Cluster', badge: '⎈' };

const navGroups = [
  {
    id: 'cluster', title: 'Cluster', items: [
      { id: 'nodes', title: 'Nodes', badge: 'ND', view: 'nodes', source: 'resource', kind: 'Node', clusterScoped: true },
      { id: 'events', title: 'Events', badge: 'EV', view: 'events', source: 'resource', kind: 'Event' },
      { id: 'namespaces', title: 'Namespaces', badge: 'NS', view: 'namespaces', source: 'resource', kind: 'Namespace', clusterScoped: true }
    ]
  },
  {
    id: 'workloads', title: 'Workloads', items: [
      { id: 'pods', title: 'Pods', badge: 'PO', source: 'pods', kind: 'Pod' },
      { id: 'deployments', title: 'Deployments', badge: 'DE', view: 'deployments', source: 'resource', kind: 'Deployment' },
      { id: 'statefulsets', title: 'StatefulSets', badge: 'SS', view: 'statefulsets', source: 'resource', kind: 'StatefulSet' },
      { id: 'daemonsets', title: 'DaemonSets', badge: 'DS', view: 'daemonsets', source: 'resource', kind: 'DaemonSet' },
      { id: 'jobs', title: 'Jobs', badge: 'JB', view: 'jobs', source: 'resource', kind: 'Job' },
      { id: 'cronjobs', title: 'CronJobs', badge: 'CJ', view: 'cronjobs', source: 'resource', kind: 'CronJob' },
      { id: 'hpas', title: 'HPAs', badge: 'HP', view: 'hpas', source: 'resource', kind: 'HorizontalPodAutoscaler' }
    ]
  },
  {
    id: 'config', title: 'Config', items: [
      { id: 'configmaps', title: 'ConfigMaps', badge: 'CM', view: 'configmaps', source: 'resource', kind: 'ConfigMap' },
      { id: 'secrets', title: 'Secrets', badge: 'SC', view: 'secrets', source: 'resource', kind: 'Secret' }
    ]
  },
  {
    id: 'network', title: 'Network', items: [
      { id: 'services', title: 'Services', badge: 'SV', view: 'services', source: 'resource', kind: 'Service' },
      { id: 'ingresses', title: 'Ingresses', badge: 'IN', view: 'ingresses', source: 'resource', kind: 'Ingress' }
    ]
  },
  {
    id: 'storage', title: 'Storage', items: [
      { id: 'pvs', title: 'Persistent Volumes', badge: 'PV', view: 'pvs', source: 'resource', kind: 'PersistentVolume', clusterScoped: true },
      { id: 'pvcs', title: 'Persistent Volume Claims', badge: 'PVC', view: 'pvcs', source: 'resource', kind: 'PersistentVolumeClaim' }
    ]
  },
  {
    id: 'custom', title: 'Custom Resources', items: [
      { id: 'crds', title: 'CRDs', badge: 'CR', view: 'crds', source: 'resource', kind: 'CustomResourceDefinition', clusterScoped: true }
    ]
  },
  {
    id: 'helm', title: 'Helm', items: [
      { id: 'helm', title: 'Releases', badge: 'HL', view: 'helm', source: 'resource', kind: 'HelmRelease' }
    ]
  }
];

const allNavItems = navGroups.flatMap((group) => group.items);
const detailKinds = new Set([
  'Pod', 'Deployment', 'StatefulSet', 'DaemonSet', 'Job', 'CronJob',
  'ConfigMap', 'Secret', 'Service', 'Ingress', 'PersistentVolumeClaim',
  'PersistentVolume', 'Namespace', 'Node', 'HorizontalPodAutoscaler',
  'CustomResourceDefinition'
]);
const editableYamlKinds = new Set([
  'Pod', 'Deployment', 'StatefulSet', 'DaemonSet', 'Job', 'CronJob',
  'ConfigMap', 'Secret', 'Service', 'Ingress', 'PersistentVolumeClaim',
  'PersistentVolume', 'Namespace', 'HorizontalPodAutoscaler',
  'CustomResourceDefinition'
]);
const podStatusFilters = ['all', 'running', 'not-running', 'pending', 'failed', 'succeeded'];
const levelFilters = ['ALL', 'INFO', 'WARN', 'ERROR', 'DEBUG'];

function findNavItem(id) {
  return allNavItems.find((item) => item.id === id) || null;
}

function h(type, props, ...children) {
  return React.createElement(type, props, ...children);
}

function classNames(...values) {
  return values.filter(Boolean).join(' ');
}

function matchesQuery(haystack, needle) {
  const query = String(needle || '').toLowerCase().trim();
  const value = String(haystack || '').toLowerCase();
  if (!query) return true;
  return value.includes(query);
}

function statusTone(value) {
  const text = String(value || '').toLowerCase();
  if (text.includes('error') || text.includes('fail') || text.includes('warning')) return 'danger';
  if (text.includes('warn') || text.includes('pending') || text.includes('suspend')) return 'warn';
  return 'success';
}

function formatAge(ns) {
  if (!ns || ns < 0) return '—';
  const totalSeconds = Math.floor(ns / 1e9);
  const days = Math.floor(totalSeconds / 86400);
  const hours = Math.floor((totalSeconds % 86400) / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  if (days > 0) return `${days}d${hours ? hours + 'h' : ''}`;
  if (hours > 0) return `${hours}h${minutes ? minutes + 'm' : ''}`;
  if (minutes > 0) return `${minutes}m`;
  return `${Math.max(totalSeconds, 0)}s`;
}

function parseLogLine(line, podName) {
  const raw = String(line || '').trimEnd();
  if (!raw) {
    return { timestamp: '—', level: 'INFO', message: '', podName, raw: '' };
  }

  // JSON structured logs.
  if (raw.startsWith('{') && raw.endsWith('}')) {
    try {
      const obj = JSON.parse(raw);
      const timestamp = obj.time || obj.timestamp || obj.ts || obj['@timestamp'] || '—';
      const level = normalizeLevel(obj.level || obj.lvl || obj.severity || obj.Severity || 'INFO');
      const message = obj.msg || obj.message || obj.MESSAGE || raw;
      return {
        timestamp: formatLogTimestamp(timestamp),
        level,
        message: String(message),
        podName,
        raw
      };
    } catch {
      // fall through
    }
  }

  // logfmt / key=value (Prometheus, Grafana, Go slog text, etc.)
  if (/(?:^|\s)(?:time|ts|timestamp|level)=/i.test(raw)) {
    const timeMatch = raw.match(/(?:^|\s)(?:time|ts|timestamp)=("(?:[^"\\]|\\.)*"|[^\s]+)/i);
    const levelMatch = raw.match(/(?:^|\s)level=("(?:[^"\\]|\\.)*"|[^\s]+)/i);
    const msgMatch = raw.match(/(?:^|\s)msg=("(?:[^"\\]|\\.)*"|[^\s]+)/i);
    const timestamp = formatLogTimestamp(stripQuotes(timeMatch?.[1]));
    const level = normalizeLevel(stripQuotes(levelMatch?.[1]) || 'INFO');
    let message = raw;
    if (msgMatch) {
      const msg = stripQuotes(msgMatch[1]);
      const rest = raw
        .replace(/(?:^|\s)(?:time|ts|timestamp)=("(?:[^"\\]|\\.)*"|[^\s]+)/ig, ' ')
        .replace(/(?:^|\s)level=("(?:[^"\\]|\\.)*"|[^\s]+)/ig, ' ')
        .replace(/(?:^|\s)msg=("(?:[^"\\]|\\.)*"|[^\s]+)/ig, ' ')
        .replace(/\s+/g, ' ')
        .trim();
      message = rest ? `${msg} ${rest}` : msg;
    } else {
      message = raw
        .replace(/(?:^|\s)(?:time|ts|timestamp)=("(?:[^"\\]|\\.)*"|[^\s]+)/ig, ' ')
        .replace(/(?:^|\s)level=("(?:[^"\\]|\\.)*"|[^\s]+)/ig, ' ')
        .replace(/\s+/g, ' ')
        .trim() || raw;
    }
    return { timestamp, level, message, podName, raw };
  }

  // Common prefixed logs: "2026-06-23T20:16:21Z INFO message" or "20:16:21 INFO message"
  const match = raw.match(/^(\d{4}-\d{2}-\d{2}[T ][^\s]+|\d{2}:\d{2}:\d{2}(?:[.,]\d{1,9})?)\s+(INFO|WARN|WARNING|ERROR|ERR|DEBUG|DBG|INF|WRN)\b\s*(.*)$/i);
  if (match) {
    return {
      timestamp: formatLogTimestamp(match[1]),
      level: normalizeLevel(match[2]),
      message: (match[3] || '').trim() || raw,
      podName,
      raw
    };
  }

  return { timestamp: '—', level: 'INFO', message: raw, podName, raw };
}

function stripQuotes(value) {
  if (value == null) return '';
  const text = String(value).trim();
  if (text.length >= 2 && text.startsWith('"') && text.endsWith('"')) {
    return text.slice(1, -1);
  }
  return text;
}

function formatLogTimestamp(value) {
  if (value == null || value === '') return '—';
  const text = String(value).trim();
  // Keep ISO-ish timestamps readable but compact.
  const iso = text.match(/^(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2})(?:\.\d+)?Z?/);
  if (iso) return `${iso[1]} ${iso[2]}`;
  return text;
}

function normalizeLevel(level) {
  const value = String(level || '').toUpperCase();
  if (value === 'WARNING' || value === 'WRN') return 'WARN';
  if (value === 'ERR') return 'ERROR';
  if (value === 'INF') return 'INFO';
  if (value === 'DBG') return 'DEBUG';
  return value || 'INFO';
}

function compactLevel(level) {
  if (level === 'INFO') return 'INF';
  if (level === 'WARN') return 'WRN';
  if (level === 'ERROR') return 'ERR';
  if (level === 'DEBUG') return 'DBG';
  return level;
}

function formatCPUMillicores(milli) {
  const value = Number(milli || 0);
  if (value < 1000) return `${Math.round(value)}m`;
  const cores = value / 1000;
  return Number.isInteger(cores) ? String(cores) : cores.toFixed(1);
}

function formatMemoryBytes(bytes) {
  const value = Number(bytes || 0);
  const ki = 1024;
  const mi = ki * 1024;
  const gi = mi * 1024;
  if (value >= gi) {
    const v = value / gi;
    return Number.isInteger(v) ? `${v}Gi` : `${v.toFixed(1)}Gi`;
  }
  if (value >= mi) {
    const v = value / mi;
    return Number.isInteger(v) ? `${v}Mi` : `${v.toFixed(1)}Mi`;
  }
  if (value >= ki) return `${Math.round(value / ki)}Ki`;
  return `${Math.round(value)}B`;
}

function metricsFromSource(source) {
  if (!source) return null;
  const cpuLabel = source.cpuLabel || source.CPULabel || '';
  const memoryLabel = source.memoryLabel || source.MemoryLabel || '';
  const diskLabel = source.diskLabel || source.DiskLabel || '';
  const networkLabel = source.networkLabel || source.NetworkLabel || '';
  const cpuPercent = source.cpuPercent ?? source.CPUPercent ?? null;
  const memoryPercent = source.memoryPercent ?? source.MemoryPercent ?? null;
  const diskPercent = source.diskPercent ?? source.DiskPercent ?? null;
  const networkPercent = source.networkPercent ?? source.NetworkPercent ?? null;
  const cpuMillicores = source.cpuMillicores ?? source.CPUMillicores ?? 0;
  const memoryBytes = source.memoryBytes ?? source.MemoryBytes ?? 0;
  const diskBytes = source.diskBytes ?? source.DiskBytes ?? 0;
  if (!cpuLabel && !memoryLabel && !diskLabel && !networkLabel && !cpuMillicores && !memoryBytes && !diskBytes) {
    return null;
  }
  return {
    cpuLabel, memoryLabel, diskLabel, networkLabel,
    cpuPercent, memoryPercent, diskPercent, networkPercent,
    cpuMillicores, memoryBytes, diskBytes
  };
}

function progressFromMetric(value) {
  if (typeof value !== 'number' || Number.isNaN(value)) return 0;
  return Math.max(0, Math.min(100, Math.round(value)));
}

function gaugeTone(percent) {
  if (percent == null) return 'muted';
  if (percent >= 90) return 'danger';
  if (percent >= 75) return 'warn';
  return 'ok';
}

function Field({ label, children }) {
  return h('div', { className: 'form-field' },
    h('span', { className: 'form-label' }, label),
    h('div', { className: 'form-value' }, children ?? '—')
  );
}

function CircularGauge({ percent, tone, size = 88, centerText }) {
  const pct = percent == null ? 0 : Math.max(0, Math.min(100, percent));
  const radius = 36;
  const circumference = 2 * Math.PI * radius;
  const dash = circumference * (pct / 100);
  const label = centerText != null
    ? centerText
    : (percent == null ? 'n/a' : `${Math.round(pct)}%`);
  const fontSize = String(label).length > 5 ? 11 : String(label).length > 3 ? 13 : 14;
  return h('svg', {
    className: classNames('circular-gauge', `tone-${tone}`),
    width: size,
    height: size,
    viewBox: '0 0 96 96',
    'aria-hidden': true
  },
    h('circle', { className: 'gauge-track', cx: 48, cy: 48, r: radius, fill: 'none', strokeWidth: 8 }),
    h('circle', {
      className: 'gauge-fill',
      cx: 48,
      cy: 48,
      r: radius,
      fill: 'none',
      strokeWidth: 8,
      strokeLinecap: 'round',
      strokeDasharray: `${dash} ${circumference}`,
      transform: 'rotate(-90 48 48)'
    }),
    h('text', {
      className: 'gauge-center-text',
      x: 48,
      y: 52,
      textAnchor: 'middle',
      style: { fontSize: `${fontSize}px` }
    }, label)
  );
}

function MetricGauge({ label, value, percent }) {
  const muted = percent == null && !value;
  const tone = gaugeTone(percent);
  return h('article', { className: classNames('lens-gauge-card', muted && 'muted-card', `tone-${tone}`) },
    h('div', { className: 'lens-gauge-visual' },
      h(CircularGauge, { percent, tone })
    ),
    h('div', { className: 'lens-gauge-meta' },
      h('span', { className: 'metric-label' }, label),
      h('strong', { className: 'metric-value' }, value || 'N/A'),
      h('div', { className: 'metric-progress' },
        h('span', {
          className: classNames('metric-progress-bar', `tone-${tone}`),
          style: { width: `${progressFromMetric(percent)}%` }
        })
      ),
      h('span', { className: 'metric-pct' },
        percent == null ? (value ? 'usage (no request/limit baseline)' : 'metrics unavailable') : `${Math.round(percent)}% of baseline`
      )
    )
  );
}

function readinessTone(ready, total) {
  if (!total) return 'muted';
  const pct = (Number(ready) / Number(total)) * 100;
  if (pct >= 100) return 'ok';
  if (pct >= 80) return 'warn';
  return 'danger';
}

/** OpenLens-style count / readiness ring. */
function CountRing({ label, value, total, tone, subtitle, onClick, percent: percentOverride, centerText }) {
  const hasTotal = total != null && Number(total) > 0;
  const percent = percentOverride != null
    ? percentOverride
    : (hasTotal ? (Number(value) / Number(total)) * 100 : (Number(value) > 0 ? 100 : 0));
  const ringTone = tone || (hasTotal ? readinessTone(value, total) : (Number(value) > 0 ? 'ok' : 'muted'));
  const center = centerText != null
    ? centerText
    : (hasTotal ? `${value}/${total}` : String(value ?? 0));
  return h('button', {
    type: 'button',
    className: classNames('count-ring-card', onClick && 'clickable'),
    onClick: onClick || undefined
  },
    h(CircularGauge, { percent, tone: ringTone, size: 92, centerText: center }),
    h('div', { className: 'count-ring-meta' },
      h('span', { className: 'count-ring-label' }, label),
      subtitle ? h('span', { className: 'count-ring-sub' }, subtitle) : null
    )
  );
}

/** Compact gauge for table cells. */
function MiniGauge({ percent, label }) {
  const tone = gaugeTone(percent);
  return h('div', { className: 'mini-gauge', title: label || '' },
    h(CircularGauge, {
      percent,
      tone,
      size: 44,
      centerText: percent == null ? '—' : `${Math.round(percent)}`
    }),
    h('span', { className: 'mini-gauge-label' }, label || 'N/A')
  );
}

function averagePercent(items, camel, pascal) {
  const values = (items || [])
    .map((item) => {
      const v = item[camel] ?? item[pascal];
      return typeof v === 'number' ? v : null;
    })
    .filter((v) => v != null && !Number.isNaN(v));
  if (!values.length) return null;
  return values.reduce((a, b) => a + b, 0) / values.length;
}

function reportingLabel(items, camel, pascal) {
  const count = (items || []).filter((item) => item[camel] || item[pascal]).length;
  return count ? `${count} node${count === 1 ? '' : 's'} reporting` : 'metrics unavailable';
}

const NS_COLLECTIONS = [
  { id: 'pods', title: 'Pods', badge: 'PO', source: 'pods' },
  { id: 'deployments', title: 'Deployments', badge: 'DE', view: 'deployments' },
  { id: 'statefulsets', title: 'StatefulSets', badge: 'SS', view: 'statefulsets' },
  { id: 'daemonsets', title: 'DaemonSets', badge: 'DS', view: 'daemonsets' },
  { id: 'jobs', title: 'Jobs', badge: 'JB', view: 'jobs' },
  { id: 'cronjobs', title: 'CronJobs', badge: 'CJ', view: 'cronjobs' },
  { id: 'hpas', title: 'HPAs', badge: 'HP', view: 'hpas' },
  { id: 'configmaps', title: 'ConfigMaps', badge: 'CM', view: 'configmaps' },
  { id: 'secrets', title: 'Secrets', badge: 'SC', view: 'secrets' },
  { id: 'services', title: 'Services', badge: 'SV', view: 'services' },
  { id: 'ingresses', title: 'Ingresses', badge: 'IN', view: 'ingresses' },
  { id: 'pvcs', title: 'PVCs', badge: 'PVC', view: 'pvcs' },
  { id: 'helm', title: 'Helm', badge: 'HL', view: 'helm' },
  { id: 'events', title: 'Events', badge: 'EV', view: 'events' }
];


function ContainerPicker({ containers, value, onChange, required = false, label = 'Container' }) {
  const multi = (containers || []).length > 1;
  const empty = !(containers || []).length;
  return h('label', { className: classNames('toolbar-field container-picker', multi && 'container-picker-required') },
    h('span', null, label, multi ? ' (required)' : ''),
    h('select', {
      value: value || '',
      disabled: empty,
      onChange: (event) => onChange(event.target.value)
    },
      empty
        ? h('option', { value: '' }, 'Loading containers…')
        : null,
      multi ? h('option', { value: '' }, 'Select a container…') : null,
      ...(containers || []).map((name) => h('option', { key: name, value: name }, name))
    ),
    required && multi && !value
      ? h('span', { className: 'picker-hint' }, 'Choose a container to continue')
      : null
  );
}

function metricsKindsFor(kind) {
  if (kind === 'Pod' || kind === 'Node') {
    return ['cpu', 'memory', 'disk', 'network'];
  }
  // Workloads can show aggregated CPU/mem when present on the row.
  if (['Deployment', 'StatefulSet', 'DaemonSet', 'Job', 'ReplicaSet'].includes(kind)) {
    return ['cpu', 'memory'];
  }
  return [];
}

function hasMetricValue(metrics, key) {
  if (!metrics) return false;
  switch (key) {
    case 'cpu':
      return Boolean(metrics.cpuLabel) || metrics.cpuPercent != null || metrics.cpuMillicores > 0;
    case 'memory':
      return Boolean(metrics.memoryLabel) || metrics.memoryPercent != null || metrics.memoryBytes > 0;
    case 'disk':
      return Boolean(metrics.diskLabel) || metrics.diskPercent != null || metrics.diskBytes > 0;
    case 'network':
      return Boolean(metrics.networkLabel) || metrics.networkPercent != null;
    default:
      return false;
  }
}

function buildOverviewFields(row) {
  if (!row) return [];
  const kind = row.kind;
  const fields = [];

  if (row.kind) fields.push({ label: 'Kind', value: row.kind });
  if (row.name) fields.push({ label: 'Name', value: row.name });
  if (row.namespace) fields.push({ label: 'Namespace', value: row.namespace });
  if (row.status) fields.push({ label: 'Status', value: row.status, chip: true });
  if (row.ageNs != null && row.ageNs !== '') fields.push({ label: 'Age', value: formatAge(row.ageNs) });

  if (kind === 'Pod') {
    if (row.ready) fields.push({ label: 'Ready', value: row.ready });
    if (row.restarts != null) fields.push({ label: 'Restarts', value: String(row.restarts) });
    if (row.raw?.Node) fields.push({ label: 'Node', value: row.raw.Node });
    if (row.raw?.Phase) fields.push({ label: 'Phase', value: row.raw.Phase, chip: true });
  }

  if (kind === 'Node') {
    // Node extras (version/os/…) are enough; avoid pod-only fields.
  }

  if (kind === 'Event') {
    // Prefer human labels for common event extras.
    const extra = row.extra || {};
    if (extra.reason) fields.push({ label: 'Reason', value: extra.reason });
    if (extra.message) fields.push({ label: 'Message', value: extra.message });
    if (extra.kind) fields.push({ label: 'Object kind', value: extra.kind });
    if (extra.count) fields.push({ label: 'Count', value: extra.count });
    return fields;
  }

  const extra = row.extra || {};
  Object.keys(extra).sort().forEach((key) => {
    const value = extra[key];
    if (value == null || value === '') return;
    fields.push({ label: key, value });
  });
  return fields;
}

function ResourceOverview({ row, yamlContent, yamlLoading }) {
  const fields = buildOverviewFields(row);
  const metrics = metricsFromSource(row) || metricsFromSource(row?.raw) || {};
  const metricKeys = (() => {
    const supported = metricsKindsFor(row?.kind);
    if (row?.kind === 'Pod' || row?.kind === 'Node') return supported;
    return supported.filter((key) => hasMetricValue(metrics, key));
  })();
  const gaugeDefs = {
    cpu: { label: 'CPU', value: metrics.cpuLabel, percent: metrics.cpuPercent ?? null },
    memory: { label: 'Memory', value: metrics.memoryLabel, percent: metrics.memoryPercent ?? null },
    disk: { label: 'Disk', value: metrics.diskLabel, percent: metrics.diskPercent ?? null },
    network: { label: 'Network', value: metrics.networkLabel, percent: metrics.networkPercent ?? null }
  };

  return h('div', { className: 'overview-panel' },
    metricKeys.length
      ? h('div', { className: classNames('drawer-metrics', metricKeys.length > 2 && 'drawer-metrics-4') },
          ...metricKeys.map((key) => h(MetricGauge, { key, ...gaugeDefs[key] }))
        )
      : null,
    fields.length
      ? h('div', { className: 'form-grid' },
          ...fields.map((field) => h(Field, { key: field.label, label: field.label },
            field.chip
              ? h('span', { className: classNames('inline-chip', `${statusTone(field.value)}-chip`) }, field.value)
              : field.value
          ))
        )
      : h('div', { className: 'empty-panel' }, 'No details available for this resource.'),
    (yamlContent || yamlLoading)
      ? h('div', { className: 'yaml-preview' },
          h('div', { className: 'detail-head' }, h('span', { className: 'eyebrow' }, 'Manifest')),
          yamlLoading
            ? h('div', { className: 'empty-panel' }, 'Loading manifest…')
            : h('pre', { className: 'detail-content compact-yaml' }, yamlContent)
        )
      : null
  );
}

function ContainersPanel({ row }) {
  const containers = row?.raw?.Containers || [];
  const images = row?.raw?.Images || [];
  if (!containers.length && !images.length) {
    return h('div', { className: 'empty-panel' }, 'No container data');
  }
  if (containers.length) {
    return h('div', { className: 'container-cards' },
      ...containers.map((ctr, index) => h('article', { key: `${ctr.Name || index}`, className: 'container-card' },
        h('div', { className: 'container-card-head' },
          h('strong', null, ctr.Name || `container-${index}`),
          h('span', { className: classNames('inline-chip', ctr.Ready ? 'success-chip' : 'warn-chip') }, ctr.Ready ? 'Ready' : 'Not ready')
        ),
        h(Field, { label: 'Image' }, ctr.Image || '—'),
        h(Field, { label: 'State' }, ctr.State || '—')
      ))
    );
  }
  return h('div', { className: 'container-cards' },
    ...images.map((image, index) => h('article', { key: `${image}-${index}`, className: 'container-card' },
      h(Field, { label: 'Image' }, image)
    ))
  );
}

function EnvPanel({ content, loading }) {
  if (loading) return h('div', { className: 'empty-panel' }, 'Loading…');
  const raw = String(content || '');
  if (!raw.trim()) return h('div', { className: 'empty-panel' }, 'No environment variables found.');
  const chunks = raw.split(/\n(?=## )/);
  return h('div', { className: 'env-panels' },
    ...chunks.map((block) => {
      const lines = block.replace(/^##\s*/, '').split('\n');
      const title = lines.shift() || 'container';
      const vars = lines.filter(Boolean).map((line) => {
        const eq = line.indexOf('=');
        if (eq === -1) return { name: line, value: '' };
        return { name: line.slice(0, eq), value: line.slice(eq + 1) };
      });
      return h('article', { key: title, className: 'env-card' },
        h('h4', null, title),
        h('div', { className: 'env-table' },
          ...vars.map((item) => h('div', { key: item.name, className: 'env-row' },
            h('code', { className: 'env-name' }, item.name),
            h('span', { className: 'env-value' }, item.value || '—')
          ))
        )
      );
    })
  );
}

function lintYamlClient(manifest) {
  const text = String(manifest || '').trim();
  if (!text) return [{ level: 'error', message: 'Manifest is empty' }];
  try {
    const doc = jsyaml.load(text);
    if (!doc || typeof doc !== 'object') return [{ level: 'error', message: 'YAML must describe a mapping object' }];
    const issues = [];
    if (!doc.apiVersion) issues.push({ level: 'error', message: 'apiVersion is required' });
    if (!doc.kind) issues.push({ level: 'error', message: 'kind is required' });
    if (!doc.metadata || !doc.metadata.name) issues.push({ level: 'error', message: 'metadata.name is required' });
    return issues;
  } catch (err) {
    const match = String(err.message || '').match(/line[: ]+(\d+)/i);
    return [{ level: 'error', line: match ? Number(match[1]) : 0, message: err.message || 'Invalid YAML' }];
  }
}

function YamlEditor({ value, loading, editable, dark, onApply, onStatus }) {
  const hostRef = useRef(null);
  const editorRef = useRef(null);
  const [draft, setDraft] = useState(value || '');
  const [issues, setIssues] = useState([]);
  const [busy, setBusy] = useState(false);
  const [dirty, setDirty] = useState(false);

  useEffect(() => {
    setDraft(value || '');
    setDirty(false);
    setIssues(lintYamlClient(value || ''));
  }, [value]);

  useEffect(() => {
    if (!hostRef.current || loading) return undefined;
    const ace = window.ace;
    if (!ace) {
      onStatus?.('YAML editor failed to load');
      return undefined;
    }

    ace.config.set('basePath', 'https://cdn.jsdelivr.net/npm/ace-builds@1.36.5/src-min-noconflict/');
    hostRef.current.innerHTML = '';
    const editor = ace.edit(hostRef.current);
    editor.setTheme(dark ? 'ace/theme/one_dark' : 'ace/theme/cloud_editor');
    editor.session.setMode('ace/mode/yaml');
    editor.setValue(value || '', -1);
    editor.setReadOnly(!editable);
    editor.setOptions({
      fontSize: 13,
      showPrintMargin: false,
      wrap: true,
      tabSize: 2,
      useSoftTabs: true,
      highlightActiveLine: Boolean(editable),
      highlightGutterLine: Boolean(editable),
      indentedSoftWrap: false
    });
    editor.renderer.setScrollMargin(8, 8, 0, 0);
    editor.clearSelection();
    editor.gotoLine(1, 0, false);

    const onChange = () => {
      const next = editor.getValue();
      setDraft(next);
      setDirty(true);
      setIssues(lintYamlClient(next));
    };
    editor.session.on('change', onChange);
    editorRef.current = editor;

    return () => {
      editor.session.off('change', onChange);
      editor.destroy();
      editorRef.current = null;
      if (hostRef.current) hostRef.current.innerHTML = '';
    };
  }, [loading, editable, dark]);

  useEffect(() => {
    const editor = editorRef.current;
    if (!editor) return;
    const current = editor.getValue();
    if ((value || '') !== current) {
      const pos = editor.getCursorPosition();
      editor.setValue(value || '', -1);
      editor.moveCursorToPosition(pos);
      editor.clearSelection();
    }
    editor.setReadOnly(!editable);
    editor.setTheme(dark ? 'ace/theme/one_dark' : 'ace/theme/cloud_editor');
  }, [value, editable, dark]);

  async function runLint(dryRun) {
    const local = lintYamlClient(draft);
    setIssues(local);
    if (local.length) {
      onStatus?.('YAML has lint issues');
      return local;
    }
    setBusy(true);
    try {
      const res = await fetch('/api/yaml/lint', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ manifest: draft, dryRun: Boolean(dryRun) })
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error || 'Lint failed');
      const next = body.issues || [];
      setIssues(next);
      onStatus?.(next.length ? `Found ${next.length} lint issue(s)` : (dryRun ? 'Dry-run passed' : 'YAML looks valid'));
      return next;
    } catch (err) {
      const next = [{ level: 'error', message: err.message || 'Lint failed' }];
      setIssues(next);
      onStatus?.(err.message || 'Lint failed');
      return next;
    } finally {
      setBusy(false);
    }
  }

  async function save() {
    const local = lintYamlClient(draft);
    setIssues(local);
    if (local.length) {
      onStatus?.('Fix lint issues before applying');
      return;
    }
    setBusy(true);
    try {
      const res = await fetch('/api/yaml/apply', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ manifest: draft })
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error || 'Apply failed');
      setDirty(false);
      onStatus?.(body.message || 'Applied');
      onApply?.(draft);
    } catch (err) {
      setIssues([{ level: 'error', message: err.message || 'Apply failed' }]);
      onStatus?.(err.message || 'Apply failed');
    } finally {
      setBusy(false);
    }
  }

  if (loading) return h('div', { className: 'empty-panel' }, 'Loading YAML…');

  return h('div', { className: 'yaml-editor' },
    h('div', { className: 'yaml-editor-toolbar' },
      h('div', { className: 'yaml-editor-meta' },
        dirty ? h('span', { className: 'chip warn' }, 'Unsaved') : h('span', { className: 'chip success' }, 'Synced'),
        issues.length
          ? h('span', { className: 'chip error' }, `${issues.length} issue${issues.length === 1 ? '' : 's'}`)
          : h('span', { className: 'chip success' }, 'Lint clean')
      ),
      h('div', { className: 'yaml-editor-actions' },
        h('button', { className: 'button ghost', disabled: busy, onClick: () => runLint(false) }, 'Lint'),
        h('button', { className: 'button ghost', disabled: busy, onClick: () => runLint(true) }, 'Dry-run'),
        editable
          ? h('button', { className: 'button primary', disabled: busy || !dirty || issues.some((i) => i.level === 'error'), onClick: save }, busy ? 'Saving…' : 'Apply')
          : null
      )
    ),
    h('div', { className: 'yaml-editor-host', ref: hostRef }),
    issues.length
      ? h('ul', { className: 'yaml-lint-list' },
          ...issues.map((issue, index) => h('li', {
            key: `${index}-${issue.message}`,
            className: classNames('yaml-lint-item', issue.level)
          }, issue.line ? `Line ${issue.line}: ${issue.message}` : issue.message))
        )
      : null
  );
}

function ShellTerminal({ namespace, pod, container }) {
  const hostRef = useRef(null);

  useEffect(() => {
    if (!hostRef.current || !namespace || !pod || !container) return undefined;

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 13,
      fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      theme: {
        background: '#0b1220',
        foreground: '#e5eefc',
        cursor: '#7dd3fc'
      }
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(hostRef.current);
    fit.fit();

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const params = new URLSearchParams({ namespace, pod, container });
    const ws = new WebSocket(`${proto}//${window.location.host}/api/exec/ws?${params.toString()}`);
    ws.binaryType = 'arraybuffer';

    const sendResize = () => {
      fit.fit();
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: 'resize', cols: term.cols, rows: term.rows }));
      }
    };

    ws.onopen = () => {
      term.writeln(`\x1b[90mshell → ${container}\x1b[0m`);
      sendResize();
    };
    ws.onmessage = (event) => {
      if (typeof event.data === 'string') {
        term.write(event.data);
        return;
      }
      term.write(new Uint8Array(event.data));
    };
    ws.onerror = () => term.writeln('\r\n\x1b[31mwebsocket error\x1b[0m');
    ws.onclose = () => term.writeln('\r\n\x1b[90m[session closed]\x1b[0m');

    const dataSub = term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) ws.send(data);
    });
    const onResize = () => sendResize();
    window.addEventListener('resize', onResize);

    return () => {
      dataSub.dispose();
      window.removeEventListener('resize', onResize);
      try { ws.close(); } catch { /* ignore */ }
      term.dispose();
    };
  }, [namespace, pod, container]);

  if (!container) {
    return h('div', { className: 'empty-panel' },
      h('h3', null, 'Select a container'),
      h('p', null, 'This pod has multiple containers. Pick one from the dropdown above to open a shell.')
    );
  }

  return h('div', { className: 'shell-panel' },
    h('div', { className: 'shell-toolbar' },
      h('span', { className: 'eyebrow' }, `Shell · ${namespace}/${pod} · ${container}`),
      h('span', { className: 'muted' }, 'Interactive xterm session')
    ),
    h('div', { className: 'xterm-host', ref: hostRef })
  );
}

function toRow(item, raw) {
  const isPod = item.source === 'pods';
  const namespace = raw.Namespace ?? '';
  const name = raw.Name ?? '';
  const metrics = metricsFromSource(raw);
  return {
    id: `${item.id}:${namespace || 'cluster'}/${name}`,
    navId: item.id,
    kind: item.kind,
    name,
    namespace,
    status: isPod ? raw.Phase : raw.Status,
    ready: raw.Ready,
    restarts: raw.Restarts,
    ageNs: raw.Age,
    extra: raw.Extra || {},
    cpuLabel: metrics?.cpuLabel || '',
    memoryLabel: metrics?.memoryLabel || '',
    diskLabel: metrics?.diskLabel || '',
    networkLabel: metrics?.networkLabel || '',
    cpuPercent: metrics?.cpuPercent ?? null,
    memoryPercent: metrics?.memoryPercent ?? null,
    diskPercent: metrics?.diskPercent ?? null,
    networkPercent: metrics?.networkPercent ?? null,
    raw
  };
}

const NAME_COL = { key: 'name', label: 'Name', value: (r) => r.name };
const NS_COL = { key: 'namespace', label: 'Namespace', value: (r) => r.namespace || '—' };
const AGE_COL = { key: 'age', label: 'Age', numeric: (r) => r.ageNs || 0, value: (r) => formatAge(r.ageNs) };

function extraCol(key, label) {
  return { key, label, value: (r) => r.extra?.[key] || '—' };
}

function statusCol(label) {
  return { key: 'status', label: label || 'Status', chip: true, value: (r) => r.status || 'Unknown' };
}

function columnsFor(navId) {
  switch (navId) {
    case 'pods':
      return [
        NAME_COL, NS_COL,
        { key: 'ready', label: 'Ready', value: (r) => r.ready || '0/0' },
        statusCol(),
        { key: 'restarts', label: 'Restarts', value: (r) => String(r.restarts ?? 0) },
        { key: 'cpu', label: 'CPU', value: (r) => r.cpuLabel || 'N/A' },
        { key: 'memory', label: 'Memory', value: (r) => r.memoryLabel || 'N/A' },
        { key: 'disk', label: 'Disk', value: (r) => r.diskLabel || 'N/A' },
        { key: 'network', label: 'Network', value: (r) => r.networkLabel || 'N/A' },
        AGE_COL
      ];
    case 'deployments':
    case 'statefulsets':
      return [NAME_COL, NS_COL, statusCol(), extraCol('images', 'Images'), AGE_COL];
    case 'daemonsets':
      return [NAME_COL, NS_COL, statusCol(), AGE_COL];
    case 'jobs':
      return [NAME_COL, NS_COL, statusCol(), extraCol('completions', 'Completions'), extraCol('active', 'Active'), extraCol('failed', 'Failed'), AGE_COL];
    case 'cronjobs':
      return [NAME_COL, NS_COL, statusCol(), extraCol('schedule', 'Schedule'), extraCol('active', 'Active runs'), AGE_COL];
    case 'hpas':
      return [NAME_COL, NS_COL, statusCol('Replicas'), extraCol('target', 'Target'), extraCol('min', 'Min'), extraCol('max', 'Max'), AGE_COL];
    case 'configmaps':
      return [NAME_COL, NS_COL, extraCol('keys', 'Keys'), AGE_COL];
    case 'secrets':
      return [NAME_COL, NS_COL, statusCol('Type'), extraCol('keys', 'Keys'), AGE_COL];
    case 'services':
      return [NAME_COL, NS_COL, statusCol('Type'), extraCol('clusterIP', 'Cluster IP'), extraCol('ports', 'Ports'), AGE_COL];
    case 'ingresses':
      return [NAME_COL, NS_COL, extraCol('hosts', 'Hosts'), extraCol('class', 'Class'), statusCol('Address'), AGE_COL];
    case 'pvcs':
      return [NAME_COL, NS_COL, statusCol(), extraCol('capacity', 'Capacity'), extraCol('storageClass', 'Storage class'), extraCol('volume', 'Volume'), AGE_COL];
    case 'pvs':
      return [NAME_COL, statusCol(), extraCol('capacity', 'Capacity'), extraCol('storageClass', 'Storage class'), extraCol('claim', 'Claim'), extraCol('reclaim', 'Reclaim'), AGE_COL];
    case 'namespaces':
      return [NAME_COL, statusCol(), extraCol('labels', 'Labels'), AGE_COL];
    case 'crds':
      return [NAME_COL, extraCol('group', 'Group'), extraCol('version', 'Version'), statusCol('Scope'), AGE_COL];
    case 'helm':
      return [NAME_COL, NS_COL, statusCol(), extraCol('version', 'Revision'), AGE_COL];
    case 'nodes':
      return [
        NAME_COL, statusCol(),
        {
          key: 'cpu',
          label: 'CPU',
          value: (r) => r.cpuLabel || 'N/A',
          numeric: (r) => r.cpuPercent ?? -1,
          cell: (r) => h(MiniGauge, { percent: r.cpuPercent, label: r.cpuLabel || 'N/A' })
        },
        {
          key: 'memory',
          label: 'Memory',
          value: (r) => r.memoryLabel || 'N/A',
          numeric: (r) => r.memoryPercent ?? -1,
          cell: (r) => h(MiniGauge, { percent: r.memoryPercent, label: r.memoryLabel || 'N/A' })
        },
        {
          key: 'disk',
          label: 'Disk',
          value: (r) => r.diskLabel || 'N/A',
          numeric: (r) => r.diskPercent ?? -1,
          cell: (r) => h(MiniGauge, { percent: r.diskPercent, label: r.diskLabel || 'N/A' })
        },
        {
          key: 'network',
          label: 'Network',
          value: (r) => r.networkLabel || 'N/A',
          numeric: (r) => r.networkPercent ?? -1,
          cell: (r) => h(MiniGauge, { percent: r.networkPercent, label: r.networkLabel || 'N/A' })
        },
        extraCol('version', 'Kubelet'), AGE_COL
      ];
    case 'events':
      return [NAME_COL, NS_COL, statusCol('Type'), extraCol('reason', 'Reason'), extraCol('message', 'Message'), AGE_COL];
    default:
      return [NAME_COL, NS_COL, statusCol(), AGE_COL];
  }
}

function App() {
  const [me, setMe] = useState(null);
  const [summary, setSummary] = useState(null);
  const [authError, setAuthError] = useState(null);
  const [contexts, setContexts] = useState([]);
  const [namespaces, setNamespaces] = useState([]);
  const [selectedNamespaces, setSelectedNamespaces] = useState([]);
  const [nsMenuOpen, setNsMenuOpen] = useState(false);
  const [nsFilter, setNsFilter] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [status, setStatus] = useState('');
  const [copied, setCopied] = useState('');

  const [theme, setTheme] = useState(() => {
    try { return window.localStorage.getItem('pyxis.theme') || ''; } catch { return ''; }
  });
  const [systemDark, setSystemDark] = useState(() => window.matchMedia('(prefers-color-scheme: dark)').matches);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [sidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [drawerWidth, setDrawerWidth] = useState(() => {
    try {
      const stored = Number(window.localStorage.getItem('pyxis.drawerWidth'));
      if (Number.isFinite(stored) && stored >= 360 && stored <= 1200) return stored;
    } catch { /* ignore */ }
    return 760;
  });
  const [drawerResizing, setDrawerResizing] = useState(false);
  const [expandedGroups, setExpandedGroups] = useState(() => Object.fromEntries(navGroups.map((g) => [g.id, true])));

  const [activeItemId, setActiveItemId] = useState('overview');
  const activeItem = useMemo(() => findNavItem(activeItemId), [activeItemId]);

  const [rows, setRows] = useState([]);
  const [rowsLoading, setRowsLoading] = useState(false);
  const [tableSearch, setTableSearch] = useState('');
  const [sortKey, setSortKey] = useState('name');
  const [sortDir, setSortDir] = useState('asc');
  const [podStatusFilter, setPodStatusFilter] = useState('all');

  const [overview, setOverview] = useState(null);
  const [nsExplore, setNsExplore] = useState(null);

  const [selectedRow, setSelectedRow] = useState(null);
  const [drawerTab, setDrawerTab] = useState('overview');
  const [drawerDetail, setDrawerDetail] = useState(null);
  const [drawerLoading, setDrawerLoading] = useState(false);
  const [containerOptions, setContainerOptions] = useState([]);
  const [selectedContainer, setSelectedContainer] = useState('');
  const [podLogs, setPodLogs] = useState([]);
  const [logsLoading, setLogsLoading] = useState(false);
  const [logFollow, setLogFollow] = useState(true);
  const [logSearch, setLogSearch] = useState('');
  const [logLevel, setLogLevel] = useState('ALL');
  const [lastLogRefresh, setLastLogRefresh] = useState(null);
  const [envContent, setEnvContent] = useState('');
  const [execOutput, setExecOutput] = useState(null);
  const [confirmState, setConfirmState] = useState(null);

  const searchRef = useRef(null);
  const logBodyRef = useRef(null);
  const shouldStickToBottomRef = useRef(true);

  useEffect(() => { bootstrap(); }, []);

  useEffect(() => {
    const stored = window.localStorage.getItem('pyxis.sidebarCollapsed');
    if (stored === 'true') setSidebarCollapsed(true);
  }, []);

  useEffect(() => {
    window.localStorage.setItem('pyxis.sidebarCollapsed', sidebarCollapsed ? 'true' : 'false');
  }, [sidebarCollapsed]);

  useEffect(() => {
    try {
      window.localStorage.setItem('pyxis.drawerWidth', String(drawerWidth));
    } catch { /* ignore */ }
  }, [drawerWidth]);

  useEffect(() => {
    if (!drawerResizing) return undefined;
    const previousCursor = document.body.style.cursor;
    const previousUserSelect = document.body.style.userSelect;
    document.body.style.cursor = 'col-resize';
    document.body.style.userSelect = 'none';

    function onMove(event) {
      const maxWidth = Math.min(1200, Math.max(360, window.innerWidth - 80));
      const next = Math.min(maxWidth, Math.max(360, window.innerWidth - event.clientX));
      setDrawerWidth(next);
    }
    function onUp() {
      setDrawerResizing(false);
    }
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    window.addEventListener('pointercancel', onUp);
    return () => {
      document.body.style.cursor = previousCursor;
      document.body.style.userSelect = previousUserSelect;
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
      window.removeEventListener('pointercancel', onUp);
    };
  }, [drawerResizing]);

  useEffect(() => {
    if (theme) {
      document.documentElement.setAttribute('data-theme', theme);
      window.localStorage.setItem('pyxis.theme', theme);
    } else {
      document.documentElement.removeAttribute('data-theme');
      window.localStorage.removeItem('pyxis.theme');
    }
  }, [theme]);

  useEffect(() => {
    const mql = window.matchMedia('(prefers-color-scheme: dark)');
    const handler = (event) => setSystemDark(event.matches);
    mql.addEventListener('change', handler);
    return () => mql.removeEventListener('change', handler);
  }, []);

  useEffect(() => {
    if (!copied) return undefined;
    const timer = window.setTimeout(() => setCopied(''), 1800);
    return () => window.clearTimeout(timer);
  }, [copied]);

  useEffect(() => {
    if (!status) return undefined;
    const timer = window.setTimeout(() => setStatus(''), 2800);
    return () => window.clearTimeout(timer);
  }, [status]);

  useEffect(() => {
    function handleKeydown(event) {
      if (event.key === 'Escape') {
        if (selectedRow) setSelectedRow(null);
        else if (nsMenuOpen) setNsMenuOpen(false);
        return;
      }
      if (event.key !== '/' || event.metaKey || event.ctrlKey || event.altKey) return;
      const tag = document.activeElement?.tagName;
      if (tag === 'INPUT' || tag === 'SELECT' || tag === 'TEXTAREA') return;
      event.preventDefault();
      searchRef.current?.focus();
    }
    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  }, [selectedRow, nsMenuOpen]);

  const effectiveTheme = theme || (systemDark ? 'dark' : 'light');

  useEffect(() => {
    if (!summary) return undefined;
    if (activeItemId === 'overview') {
      loadOverview();
    } else if (activeItem) {
      loadItemRows(activeItem);
    }
    return undefined;
  }, [summary, activeItemId, podStatusFilter]);

  useEffect(() => {
    if (!selectedRow) return undefined;
    setDrawerTab('overview');
    setExecOutput(null);
    return undefined;
  }, [selectedRow?.id]);

  useEffect(() => {
    if (!selectedRow) { setDrawerDetail(null); return undefined; }
    if (drawerTab === 'overview' || drawerTab === 'yaml') loadDrawerOverview(selectedRow);
    if (drawerTab === 'env' && selectedRow.kind === 'Pod') loadPodEnv(selectedRow);
    return undefined;
  }, [selectedRow?.id, drawerTab]);

  useEffect(() => {
    if (!selectedRow || selectedRow.kind !== 'Pod') {
      setContainerOptions([]);
      setSelectedContainer('');
      return undefined;
    }
    (async () => {
      try {
        const res = await fetch(`/api/pod-containers?namespace=${encodeURIComponent(selectedRow.namespace)}&pod=${encodeURIComponent(selectedRow.name)}`, { credentials: 'include' });
        const body = await res.json();
        const items = body.items || [];
        setContainerOptions(items);
        // Single container: auto-select. Multiple: force explicit dropdown choice.
        setSelectedContainer(items.length === 1 ? items[0] : '');
      } catch {
        setContainerOptions([]);
        setSelectedContainer('');
      }
    })();
    return undefined;
  }, [selectedRow?.id]);

  useEffect(() => {
    if (!selectedRow || selectedRow.kind !== 'Pod' || drawerTab !== 'logs') return undefined;
    if (!selectedContainer) {
      setPodLogs([]);
      return undefined;
    }
    refreshPodLogs(selectedRow);
    if (!logFollow) return undefined;
    const timer = window.setInterval(() => refreshPodLogs(selectedRow, { silent: true }), 3000);
    return () => window.clearInterval(timer);
  }, [selectedRow?.id, drawerTab, selectedContainer, logFollow]);

  useEffect(() => {
    if (!logBodyRef.current || !shouldStickToBottomRef.current) return;
    logBodyRef.current.scrollTop = logBodyRef.current.scrollHeight;
  }, [podLogs]);

  const namespaceOptions = useMemo(() => namespaces.filter((namespace) => matchesQuery(namespace, nsFilter)), [namespaces, nsFilter]);

  const filteredRows = useMemo(() => {
    let list = rows;
    if (selectedNamespaces.length) {
      list = list.filter((row) => !row.namespace || selectedNamespaces.includes(row.namespace));
    }
    if (tableSearch) {
      list = list.filter((row) => matchesQuery(`${row.name} ${row.namespace} ${row.status} ${JSON.stringify(row.extra)}`, tableSearch));
    }
    return list;
  }, [rows, selectedNamespaces, tableSearch]);

  const columns = useMemo(() => columnsFor(activeItemId), [activeItemId]);

  const sortedRows = useMemo(() => {
    const column = columns.find((c) => c.key === sortKey) || columns[0];
    const dir = sortDir === 'asc' ? 1 : -1;
    return [...filteredRows].sort((a, b) => {
      if (column.numeric) return (column.numeric(a) - column.numeric(b)) * dir;
      const av = String(column.value(a) ?? '').toLowerCase();
      const bv = String(column.value(b) ?? '').toLowerCase();
      return av.localeCompare(bv) * dir;
    });
  }, [filteredRows, columns, sortKey, sortDir]);

  const filteredLogs = useMemo(() => podLogs.filter((entry) => {
    const levelMatch = logLevel === 'ALL' || entry.level === logLevel;
    const searchMatch = !logSearch || matchesQuery(entry.raw, logSearch);
    return levelMatch && searchMatch;
  }), [podLogs, logLevel, logSearch]);

  async function bootstrap() {
    setLoading(true);
    setError('');
    try {
      const meResp = await fetch('/api/me', { credentials: 'include' });
      if (meResp.status === 401) {
        setAuthError(await meResp.json());
        setLoading(false);
        return;
      }
      const meBody = await meResp.json();
      const [summaryResp, nsResp, contextsResp] = await Promise.all([
        fetch('/api/summary', { credentials: 'include' }),
        fetch('/api/namespaces', { credentials: 'include' }),
        fetch('/api/contexts', { credentials: 'include' })
      ]);
      const summaryBody = await summaryResp.json();
      const namespaceBody = await nsResp.json();
      const contextsBody = await contextsResp.json();
      setMe(meBody);
      setSummary(summaryBody);
      setContexts(contextsBody.items || []);
      setNamespaces(namespaceBody.items || []);
      setAuthError(null);
    } catch (err) {
      setError(err.message || 'Failed to load Pyxis Web.');
    } finally {
      setLoading(false);
    }
  }

  async function loadOverview() {
    setRowsLoading(true);
    setError('');
    try {
      const views = [
        'nodes', 'events', 'deployments', 'statefulsets', 'daemonsets',
        'jobs', 'services', 'ingresses', 'pvcs', 'hpas'
      ];
      const [podsResp, ...viewResps] = await Promise.all([
        fetch('/api/pods', { credentials: 'include' }),
        ...views.map((view) => fetch(`/api/resources?view=${view}`, { credentials: 'include' }))
      ]);
      const podsBody = await podsResp.json();
      const bodies = await Promise.all(viewResps.map((res) => res.json()));
      const byView = Object.fromEntries(views.map((view, index) => [view, bodies[index].items || []]));
      const podItems = podsBody.items || [];
      const nodeItems = byView.nodes || [];
      const eventItems = byView.events || [];
      const phaseCounts = podItems.reduce((acc, pod) => {
        const phase = String(pod.Phase || 'Unknown');
        acc[phase] = (acc[phase] || 0) + 1;
        return acc;
      }, {});
      const runningPods = phaseCounts.Running || 0;
      const failedPods = (phaseCounts.Failed || 0) + (phaseCounts.Pending || 0);
      setOverview({
        nodeCount: nodeItems.length,
        nodesReady: nodeItems.filter((n) => n.Status === 'Ready').length,
        namespaceCount: namespaces.length,
        podCount: podItems.length,
        runningPods,
        failedPods,
        phaseCounts,
        warningEvents: eventItems.filter((e) => e.Status === 'Warning').length,
        eventCount: eventItems.length,
        counts: {
          deployments: (byView.deployments || []).length,
          statefulsets: (byView.statefulsets || []).length,
          daemonsets: (byView.daemonsets || []).length,
          jobs: (byView.jobs || []).length,
          services: (byView.services || []).length,
          ingresses: (byView.ingresses || []).length,
          pvcs: (byView.pvcs || []).length,
          hpas: (byView.hpas || []).length
        },
        capacity: {
          cpuPercent: averagePercent(nodeItems, 'cpuPercent', 'CPUPercent'),
          memoryPercent: averagePercent(nodeItems, 'memoryPercent', 'MemoryPercent'),
          diskPercent: averagePercent(nodeItems, 'diskPercent', 'DiskPercent'),
          cpuLabel: reportingLabel(nodeItems, 'cpuLabel', 'CPULabel'),
          memoryLabel: reportingLabel(nodeItems, 'memoryLabel', 'MemoryLabel'),
          diskLabel: reportingLabel(nodeItems, 'diskLabel', 'DiskLabel')
        }
      });
    } catch (err) {
      setError(err.message || 'Failed to load cluster overview.');
    } finally {
      setRowsLoading(false);
    }
  }

  async function openNamespaceExplorer(namespaceName) {
    setSelectedRow(null);
    setNsExplore({ name: namespaceName, loading: true, collections: [] });
    setError('');
    try {
      const collections = await Promise.all(NS_COLLECTIONS.map(async (collection) => {
        try {
          let items = [];
          if (collection.source === 'pods') {
            const res = await fetch(`/api/pods?namespace=${encodeURIComponent(namespaceName)}`, { credentials: 'include' });
            const body = await res.json();
            if (!res.ok) throw new Error(body.error || 'Failed to load pods');
            items = body.items || [];
          } else {
            const res = await fetch(`/api/resources?view=${encodeURIComponent(collection.view)}&namespace=${encodeURIComponent(namespaceName)}`, { credentials: 'include' });
            const body = await res.json();
            if (!res.ok) throw new Error(body.error || `Failed to load ${collection.title}`);
            items = body.items || [];
          }
          return {
            ...collection,
            count: items.length,
            items: items.slice(0, 8).map((entry) => ({
              name: entry.Name || entry.name,
              status: entry.Status || entry.Phase || entry.status || ''
            }))
          };
        } catch {
          return { ...collection, count: 0, items: [], error: true };
        }
      }));
      setNsExplore({ name: namespaceName, loading: false, collections });
    } catch (err) {
      setError(err.message || 'Failed to load namespace resources.');
      setNsExplore({ name: namespaceName, loading: false, collections: [] });
    }
  }

  function openNamespaceCollection(collectionId) {
    if (!nsExplore?.name) return;
    setSelectedNamespaces([nsExplore.name]);
    setNsExplore(null);
    selectNavItem(collectionId);
  }

  async function loadItemRows(item) {
    setRowsLoading(true);
    setError('');
    try {
      let raw;
      if (item.source === 'pods') {
        const res = await fetch(`/api/pods?filter=${encodeURIComponent(podStatusFilter)}`, { credentials: 'include' });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || 'Failed to load pods');
        raw = body.items || [];
      } else {
        const res = await fetch(`/api/resources?view=${encodeURIComponent(item.view)}`, { credentials: 'include' });
        const body = await res.json();
        if (!res.ok) throw new Error(body.error || `Failed to load ${item.title}`);
        raw = body.items || [];
      }
      setRows(raw.map((entry) => toRow(item, entry)));
    } catch (err) {
      setError(err.message || 'Failed to load resources.');
      setRows([]);
    } finally {
      setRowsLoading(false);
    }
  }

  async function loadDrawerOverview(row) {
    // Structured overview is rendered from the row; only fetch YAML/manifest when available.
    if (!detailKinds.has(row.kind)) {
      setDrawerDetail(null);
      return;
    }
    setDrawerLoading(true);
    try {
      const res = await fetch(`/api/detail?kind=${encodeURIComponent(row.kind)}&namespace=${encodeURIComponent(row.namespace)}&name=${encodeURIComponent(row.name)}`, { credentials: 'include' });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error || 'Failed to load detail');
      setDrawerDetail(body);
    } catch (err) {
      setError(err.message || 'Failed to load resource detail.');
      setDrawerDetail(null);
    } finally {
      setDrawerLoading(false);
    }
  }

  async function loadPodEnv(row) {
    setDrawerLoading(true);
    try {
      const res = await fetch(`/api/pod-env?namespace=${encodeURIComponent(row.namespace)}&pod=${encodeURIComponent(row.name)}`, { credentials: 'include' });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error || 'Failed to load environment variables');
      const content = (body.items || []).map((container) => {
        const envLines = (container.EnvVars || []).map((envVar) => `${envVar.Name}=${envVar.Value || envVar.ValueFrom || ''}`).join('\n');
        return `## ${container.Name} (${container.Image})\n${envLines || '(no env vars found)'}`;
      }).join('\n\n');
      setEnvContent(content || 'No environment variables found.');
    } catch (err) {
      setError(err.message || 'Failed to load environment variables.');
    } finally {
      setDrawerLoading(false);
    }
  }

  async function refreshPodLogs(row, options = {}) {
    if (!selectedContainer) {
      setPodLogs([]);
      return;
    }
    if (!options.silent) setLogsLoading(true);
    try {
      const params = new URLSearchParams({
        namespace: row.namespace,
        pod: row.name,
        container: selectedContainer,
        tail: '200'
      });
      const res = await fetch(`/api/pod-logs?${params.toString()}`, { credentials: 'include' });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error || 'Failed to load logs');
      setPodLogs(String(body.content || '').split('\n').filter(Boolean).map((line) => parseLogLine(line, row.name)));
      setLastLogRefresh(new Date());
    } catch (err) {
      setError(err.message || 'Failed to load logs.');
    } finally {
      if (!options.silent) setLogsLoading(false);
    }
  }

  async function switchCluster(contextName) {
    if (!contextName || contextName === summary?.currentContext) return;
    setStatus(`Switching cluster to ${contextName}…`);
    try {
      const response = await fetch('/api/context', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ context: contextName })
      });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || 'Failed to switch context');
      setSummary((current) => ({ ...current, currentContext: contextName }));
      setStatus(body.message || `Switched to context: ${contextName}`);
      setSelectedRow(null);
      await bootstrap();
    } catch (err) {
      setError(err.message || 'Failed to switch context.');
    }
  }

  async function runAction(action, extra = {}) {
    if (!selectedRow && action !== 'create') return;
    const kind = extra.kind || selectedRow?.kind;
    const payload = {
      action,
      kind,
      namespace: extra.namespace ?? selectedRow?.namespace ?? '',
      name: extra.name ?? selectedRow?.name ?? '',
      container: selectedContainer || '',
      ...extra
    };
    setStatus(`${action}…`);
    try {
      const response = await fetch('/api/actions', {
        method: 'POST',
        credentials: 'include',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      const body = await response.json();
      if (!response.ok) throw new Error(body.error || `${action} failed`);
      setStatus(body.message || `${action} ok`);
      if (action === 'delete') {
        setSelectedRow(null);
      }
      if (['delete', 'restart', 'scale', 'create'].includes(action) && activeItem) {
        await loadItemRows(activeItem);
      }
      if ((action === 'create' || action === 'delete') && (kind === 'Namespace' || selectedRow?.kind === 'Namespace')) {
        const nsRes = await fetch('/api/namespaces', { credentials: 'include' });
        const nsBody = await nsRes.json();
        if (nsRes.ok) setNamespaces(nsBody.items || []);
      }
      if (action === 'exec' && body.message) {
        setExecOutput(body.message);
      }
    } catch (err) {
      setError(err.message || `${action} failed`);
    }
  }

  async function createNamespace() {
    const name = window.prompt('Create namespace');
    if (!name || !String(name).trim()) return;
    await runAction('create', { kind: 'Namespace', name: String(name).trim(), namespace: '' });
  }

  function requestConfirm(opts) {
    setConfirmState(opts);
  }

  async function handleConfirm() {
    const active = confirmState;
    setConfirmState(null);
    if (active?.onConfirm) await active.onConfirm();
  }

  async function copyText(value) {
    if (!value) return;
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
    } else {
      const area = document.createElement('textarea');
      area.value = value;
      document.body.appendChild(area);
      area.select();
      document.execCommand('copy');
      area.remove();
    }
    setCopied('Copied');
  }

  function handleLogScroll(event) {
    const element = event.currentTarget;
    const nearBottom = element.scrollHeight - element.scrollTop - element.clientHeight < 24;
    shouldStickToBottomRef.current = nearBottom;
  }

  function toggleTheme() {
    setTheme(effectiveTheme === 'dark' ? 'light' : 'dark');
  }

  function toggleSidebar() {
    if (window.innerWidth <= 1024) {
      setSidebarOpen((current) => !current);
      return;
    }
    setSidebarCollapsed((current) => !current);
  }

  function toggleGroup(groupId) {
    setExpandedGroups((current) => ({ ...current, [groupId]: !current[groupId] }));
  }

  function selectNavItem(id) {
    setActiveItemId(id);
    setSelectedRow(null);
    setNsExplore(null);
    setTableSearch('');
    setSortKey('name');
    setSortDir('asc');
    setSidebarOpen(false);
  }

  function toggleNamespace(namespace) {
    setSelectedNamespaces((current) => (
      current.includes(namespace) ? current.filter((n) => n !== namespace) : [...current, namespace]
    ));
  }

  function toggleSort(key) {
    if (sortKey === key) {
      setSortDir((current) => (current === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortKey(key);
      setSortDir('asc');
    }
  }

  function confirmDelete() {
    requestConfirm({
      title: `Delete ${selectedRow.kind}`,
      message: `Delete ${selectedRow.kind.toLowerCase()} "${selectedRow.name}" in ${selectedRow.namespace || 'cluster scope'}? This cannot be undone.`,
      confirmLabel: 'Delete',
      onConfirm: () => runAction('delete')
    });
  }

  function confirmRestart() {
    requestConfirm({
      title: `Restart ${selectedRow.kind}`,
      message: `Restart ${selectedRow.kind.toLowerCase()} "${selectedRow.name}"? This will roll all its pods.`,
      confirmLabel: 'Restart',
      onConfirm: () => runAction('restart')
    });
  }

  if (loading) {
    return h('div', { className: 'shell centered' },
      h('div', { className: 'loading-card' },
        h('span', { className: 'spinner' }),
        h('p', null, 'Loading Pyxis…')
      )
    );
  }

  if (authError) {
    return h('div', { className: 'shell centered' },
      h('div', { className: 'auth-card' },
        h('span', { className: 'eyebrow' }, 'Protected workspace'),
        h('h1', null, 'Pyxis'),
        h('p', null, 'Sign in with Dex to inspect cluster resources and stream logs in the browser.'),
        h('a', { className: 'button primary', href: authError.loginUrl || '/login' }, 'Continue with Dex')
      )
    );
  }

  const nsLabel = selectedNamespaces.length === 0
    ? 'All namespaces'
    : selectedNamespaces.length === 1
      ? selectedNamespaces[0]
      : `${selectedNamespaces.length} namespaces`;

  const sidebar = h('aside', { className: classNames('sidebar', sidebarOpen && 'open', sidebarCollapsed && 'collapsed') },
    h('div', { className: 'sidebar-head' },
      h('div', { className: 'brand' },
        h('span', { className: 'brand-mark' }, '⎈'),
        sidebarCollapsed ? null : h('span', { className: 'brand-name' }, 'Pyxis')
      ),
      h('div', { className: 'sidebar-head-actions' },
        h('button', {
          className: 'icon-button theme-toggle',
          onClick: toggleTheme,
          title: effectiveTheme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme',
          'aria-label': 'Toggle color theme'
        }, effectiveTheme === 'dark' ? '☀' : '☾'),
        h('button', { className: 'icon-button sidebar-toggle', onClick: toggleSidebar, title: sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar', 'aria-label': sidebarCollapsed ? 'Expand sidebar' : 'Collapse sidebar' }, sidebarCollapsed ? '›' : '‹'),
        h('button', { className: 'icon-button sidebar-close', onClick: () => setSidebarOpen(false), title: 'Close sidebar', 'aria-label': 'Close sidebar' }, '✕')
      )
    ),
    h('section', { className: 'sidebar-section' },
      h('label', { className: 'field-label', htmlFor: 'cluster-switcher' }, 'Cluster'),
      h('select', {
        id: 'cluster-switcher',
        value: summary?.currentContext || '',
        onChange: (event) => switchCluster(event.target.value)
      }, ...contexts.map((contextName) => h('option', { key: contextName, value: contextName }, contextName)))
    ),
    h('nav', { className: 'nav-scroll' },
      h('button', {
        className: classNames('nav-item nav-item-top', activeItemId === 'overview' && 'active'),
        onClick: () => selectNavItem('overview')
      },
        h('span', { className: 'nav-badge' }, overviewItem.badge),
        h('span', { className: 'nav-label' }, overviewItem.title)
      ),
      ...navGroups.map((group) => h('div', { key: group.id, className: 'nav-group' },
        h('button', { className: 'nav-group-title', onClick: () => toggleGroup(group.id) },
          h('span', { className: 'tree-chevron' }, expandedGroups[group.id] ? '▾' : '▸'),
          h('span', null, group.title)
        ),
        expandedGroups[group.id] ? h('div', { className: 'nav-group-items' },
          ...group.items.map((item) => h('button', {
            key: item.id,
            className: classNames('nav-item', activeItemId === item.id && 'active'),
            onClick: () => selectNavItem(item.id)
          },
            h('span', { className: 'nav-badge' }, item.badge),
            h('span', { className: 'nav-label' }, item.title)
          ))
        ) : null
      ))
    )
  );

  const topbar = h('header', { className: 'app-topbar' },
    h('div', { className: 'app-topbar-left' },
      h('button', { className: 'icon-button menu-button', onClick: () => setSidebarOpen(true), title: 'Open sidebar', 'aria-label': 'Open sidebar' }, '☰'),
      h('div', { className: 'title-stack' },
        h('span', { className: 'resource-eyebrow' }, `${summary?.currentContext || 'cluster'}`),
        h('h2', null,
          activeItemId === 'overview'
            ? 'Cluster overview'
            : (nsExplore ? `Namespace · ${nsExplore.name}` : (activeItem?.title || 'Select a view'))
        )
      )
    ),
    activeItemId !== 'overview' ? h('div', { className: 'app-topbar-right' },
      h('div', { className: 'ns-dropdown' },
        h('button', { className: 'button ghost ns-dropdown-trigger', onClick: () => setNsMenuOpen((v) => !v) },
          nsLabel, h('span', { className: 'tree-chevron' }, '▾')
        ),
        nsMenuOpen ? h(React.Fragment, null,
          h('div', { className: 'ns-dropdown-backdrop', onClick: () => setNsMenuOpen(false) }),
          h('div', { className: 'ns-dropdown-panel' },
            h('input', {
              value: nsFilter,
              onChange: (event) => setNsFilter(event.target.value),
              placeholder: 'Filter namespaces…',
              autoFocus: true
            }),
            h('button', { className: 'ns-option ns-option-all', onClick: () => setSelectedNamespaces([]) },
              h('span', null, 'All namespaces'),
              selectedNamespaces.length === 0 ? h('span', null, '✓') : null
            ),
            h('div', { className: 'ns-option-list' },
              ...namespaceOptions.map((namespace) => h('button', {
                key: namespace,
                className: 'ns-option',
                onClick: () => toggleNamespace(namespace)
              },
                h('span', null, namespace),
                selectedNamespaces.includes(namespace) ? h('span', null, '✓') : null
              ))
            )
          )
        ) : null
      ),
      h('div', { className: 'search-field topbar-search' },
        h('input', {
          ref: searchRef,
          value: tableSearch,
          onChange: (event) => setTableSearch(event.target.value),
          placeholder: `Search ${activeItem?.title || 'resources'}…`
        }),
        h('span', { className: 'kbd-hint' }, '/')
      ),
      activeItem?.source === 'pods' ? h('select', { value: podStatusFilter, onChange: (event) => setPodStatusFilter(event.target.value) },
        ...podStatusFilters.map((filter) => h('option', { key: filter, value: filter }, filter))
      ) : null,
      h('button', { className: 'button ghost', onClick: () => (activeItemId === 'overview' ? loadOverview() : loadItemRows(activeItem)) }, 'Refresh')
    ) : null
  );

  const feedback = (copied || status || error) ? h('div', { className: 'feedback-row' },
    copied ? h('span', { className: 'inline-chip info-chip' }, copied) : null,
    status ? h('span', { className: 'inline-chip success-chip' }, status) : null,
    error ? h('span', { className: 'inline-chip error-chip' }, error) : null
  ) : null;

  let pageContent;
  if (activeItemId === 'overview') {
    const counts = overview?.counts || {};
    const capacity = overview?.capacity || {};
    const phases = overview?.phaseCounts || {};
    pageContent = h('section', { className: 'overview-page' },
      rowsLoading ? h('div', { className: 'table-loading' }, h('span', { className: 'spinner small' })) : null,
      h('div', { className: 'overview-hero' },
        h('div', null,
          h('span', { className: 'eyebrow' }, 'Cluster overview'),
          h('h2', null, summary?.currentContext || 'cluster'),
          h('p', { className: 'muted' }, `${summary?.serverVersion || 'unknown'} · ${me?.name || me?.email || 'local'}`)
        ),
        h('div', { className: 'overview-hero-stats' },
          h('span', { className: 'inline-chip info-chip' }, `${overview?.namespaceCount ?? '—'} namespaces`),
          h('span', { className: classNames('inline-chip', overview?.warningEvents ? 'warn-chip' : 'success-chip') },
            `${overview?.warningEvents ?? 0} warnings`)
        )
      ),
      h('div', { className: 'overview-section' },
        h('h3', { className: 'overview-section-title' }, 'Cluster health'),
        h('div', { className: 'count-ring-grid' },
          h(CountRing, {
            label: 'Nodes',
            value: overview?.nodesReady ?? 0,
            total: overview?.nodeCount ?? 0,
            subtitle: 'ready',
            onClick: () => selectNavItem('nodes')
          }),
          h(CountRing, {
            label: 'Pods',
            value: overview?.runningPods ?? 0,
            total: overview?.podCount ?? 0,
            percent: (() => {
              const total = overview?.podCount || 0;
              if (!total) return 0;
              const healthy = (overview?.runningPods || 0) + (phases.Succeeded || 0);
              return (healthy / total) * 100;
            })(),
            tone: (() => {
              const pending = phases.Pending || 0;
              const failed = phases.Failed || 0;
              const unknown = phases.Unknown || 0;
              if (failed > 0) return 'danger';
              if (pending > 0 || unknown > 0) return 'warn';
              return 'ok';
            })(),
            subtitle: 'running',
            onClick: () => selectNavItem('pods')
          }),
          h(CountRing, {
            label: 'Namespaces',
            value: overview?.namespaceCount ?? 0,
            subtitle: 'in cluster',
            onClick: () => selectNavItem('namespaces')
          }),
          h(CountRing, {
            label: 'Events',
            value: overview?.warningEvents ?? 0,
            total: Math.max(overview?.eventCount || 0, overview?.warningEvents || 0) || undefined,
            tone: overview?.warningEvents ? 'warn' : 'ok',
            subtitle: 'warnings',
            onClick: () => selectNavItem('events')
          })
        )
      ),
      h('div', { className: 'overview-section' },
        h('h3', { className: 'overview-section-title' }, 'Workloads & networking'),
        h('div', { className: 'count-ring-grid' },
          h(CountRing, { label: 'Deployments', value: counts.deployments || 0, onClick: () => selectNavItem('deployments') }),
          h(CountRing, { label: 'StatefulSets', value: counts.statefulsets || 0, onClick: () => selectNavItem('statefulsets') }),
          h(CountRing, { label: 'DaemonSets', value: counts.daemonsets || 0, onClick: () => selectNavItem('daemonsets') }),
          h(CountRing, { label: 'Jobs', value: counts.jobs || 0, onClick: () => selectNavItem('jobs') }),
          h(CountRing, { label: 'Services', value: counts.services || 0, onClick: () => selectNavItem('services') }),
          h(CountRing, { label: 'Ingresses', value: counts.ingresses || 0, onClick: () => selectNavItem('ingresses') }),
          h(CountRing, { label: 'PVCs', value: counts.pvcs || 0, onClick: () => selectNavItem('pvcs') }),
          h(CountRing, { label: 'HPAs', value: counts.hpas || 0, onClick: () => selectNavItem('hpas') })
        )
      ),
      h('div', { className: 'overview-section' },
        h('h3', { className: 'overview-section-title' }, 'Pod phases'),
        h('div', { className: 'count-ring-grid phase-grid' },
          ...['Running', 'Pending', 'Failed', 'Succeeded', 'Unknown'].map((phase) => h(CountRing, {
            key: phase,
            label: phase,
            value: phases[phase] || 0,
            total: overview?.podCount || undefined,
            tone: phase === 'Failed' ? (phases[phase] ? 'danger' : 'muted')
              : phase === 'Pending' ? (phases[phase] ? 'warn' : 'muted')
                : phase === 'Running' ? 'ok' : 'muted',
            onClick: () => selectNavItem('pods')
          }))
        )
      ),
      h('div', { className: 'overview-section' },
        h('h3', { className: 'overview-section-title' }, 'Cluster capacity'),
        h('div', { className: 'drawer-metrics drawer-metrics-4 overview-capacity' },
          h(MetricGauge, { label: 'CPU', value: capacity.cpuLabel, percent: capacity.cpuPercent }),
          h(MetricGauge, { label: 'Memory', value: capacity.memoryLabel, percent: capacity.memoryPercent }),
          h(MetricGauge, { label: 'Disk', value: capacity.diskLabel, percent: capacity.diskPercent })
        )
      )
    );
  } else if (activeItemId === 'namespaces' && nsExplore) {
    pageContent = h('section', { className: 'ns-explore' },
      h('div', { className: 'table-toolbar' },
        h('div', { className: 'table-toolbar-copy' },
          h('button', { className: 'button ghost', onClick: () => setNsExplore(null) }, '← Back'),
          h('strong', null, nsExplore.name),
          h('span', { className: 'muted' }, 'All collections in this namespace')
        ),
        h('button', {
          className: 'button ghost',
          onClick: () => openNamespaceExplorer(nsExplore.name)
        }, 'Refresh')
      ),
      nsExplore.loading
        ? h('div', { className: 'table-loading' }, h('span', { className: 'spinner small' }))
        : h('div', { className: 'ns-explore-body' },
            h('div', { className: 'count-ring-grid' },
              ...(nsExplore.collections || []).map((collection) => h(CountRing, {
                key: collection.id,
                label: collection.title,
                value: collection.count,
                subtitle: collection.error ? 'unavailable' : `${collection.count} item${collection.count === 1 ? '' : 's'}`,
                tone: collection.error ? 'danger' : undefined,
                onClick: () => openNamespaceCollection(collection.id)
              }))
            ),
            h('div', { className: 'ns-collection-lists' },
              ...(nsExplore.collections || []).filter((c) => c.count > 0).map((collection) => h('article', {
                key: `${collection.id}-list`,
                className: 'ns-collection-card'
              },
                h('header', { className: 'ns-collection-head' },
                  h('button', {
                    className: 'ns-collection-title',
                    onClick: () => openNamespaceCollection(collection.id)
                  }, h('span', { className: 'nav-badge' }, collection.badge), collection.title),
                  h('span', { className: 'muted' }, String(collection.count))
                ),
                h('ul', { className: 'ns-collection-items' },
                  ...collection.items.map((item) => h('li', { key: item.name },
                    h('span', null, item.name),
                    item.status ? h('span', { className: classNames('inline-chip', `${statusTone(item.status)}-chip`) }, item.status) : null
                  )),
                  collection.count > collection.items.length
                    ? h('li', { className: 'muted' }, `+${collection.count - collection.items.length} more`)
                    : null
                )
              ))
            ),
            !(nsExplore.collections || []).some((c) => c.count > 0)
              ? h('div', { className: 'empty-panel' },
                  h('h3', null, 'Empty namespace'),
                  h('p', null, 'No workloads, config, or network resources found here.')
                )
              : null
          )
    );
  } else if (!activeItem) {
    pageContent = h('div', { className: 'empty-panel' }, h('h3', null, 'Select a resource type from the sidebar'));
  } else {
    pageContent = h('section', { className: 'resource-table-card' },
      activeItemId === 'namespaces'
        ? h('div', { className: 'table-toolbar' },
            h('div', { className: 'table-toolbar-copy' },
              h('strong', null, 'Namespace manager'),
              h('span', { className: 'muted' }, 'Click a namespace to browse all of its resources')
            ),
            h('button', { className: 'button primary', onClick: createNamespace }, 'Create namespace')
          )
        : null,
      rowsLoading ? h('div', { className: 'table-loading' }, h('span', { className: 'spinner small' })) : null,
      h('div', { className: 'table-scroll' },
        h('table', { className: 'resource-table' },
          h('thead', null,
            h('tr', null,
              ...columns.map((col) => h('th', {
                key: col.key,
                onClick: () => toggleSort(col.key),
                className: sortKey === col.key ? 'sorted' : ''
              }, col.label, sortKey === col.key ? h('span', { className: 'sort-arrow' }, sortDir === 'asc' ? ' ▲' : ' ▼') : null))
            )
          ),
          h('tbody', null,
            sortedRows.length ? sortedRows.map((row) => h('tr', {
              key: row.id,
              className: classNames('table-row', selectedRow?.id === row.id && 'active'),
              onClick: () => {
                if (row.kind === 'Namespace') {
                  openNamespaceExplorer(row.name);
                  return;
                }
                setSelectedRow(row);
              }
            },
              ...columns.map((col) => h('td', { key: col.key },
                col.cell
                  ? col.cell(row)
                  : col.chip
                    ? h('span', { className: classNames('inline-chip', `${statusTone(col.value(row))}-chip`) }, col.value(row))
                    : col.value(row)
              ))
            )) : null
          )
        ),
        !rowsLoading && sortedRows.length === 0 ? h('div', { className: 'empty-panel' },
          h('h3', null, 'No resources found'),
          h('p', null, tableSearch || selectedNamespaces.length ? 'Try clearing the search or namespace filter.' : 'This cluster has none of this resource kind.')
        ) : null
      )
    );
  }

  const drawer = selectedRow ? h('div', { className: 'drawer-wrap' },
    h('div', { className: 'drawer-backdrop', onClick: () => setSelectedRow(null) }),
    h('aside', {
      className: classNames('drawer', drawerResizing && 'resizing'),
      style: { '--drawer-width': `${drawerWidth}px` }
    },
      h('div', {
        className: classNames('drawer-resizer', drawerResizing && 'active'),
        role: 'separator',
        'aria-orientation': 'vertical',
        'aria-label': 'Resize details panel',
        title: 'Drag to resize',
        onPointerDown: (event) => {
          event.preventDefault();
          event.stopPropagation();
          setDrawerResizing(true);
        }
      }),
      h('div', { className: 'drawer-head' },
        h('div', null,
          h('span', { className: 'eyebrow' }, selectedRow.kind),
          h('h3', null, selectedRow.name),
          selectedRow.namespace ? h('span', { className: 'resource-eyebrow' }, selectedRow.namespace) : null
        ),
        h('button', { className: 'icon-button', onClick: () => setSelectedRow(null), 'aria-label': 'Close' }, '✕')
      ),
      h('div', { className: 'tab-row drawer-tabs' },
        h('button', { className: classNames('tab-button', drawerTab === 'overview' && 'active'), onClick: () => setDrawerTab('overview') }, 'Overview'),
        selectedRow.kind === 'Pod' ? h('button', { className: classNames('tab-button', drawerTab === 'containers' && 'active'), onClick: () => setDrawerTab('containers') }, 'Containers') : null,
        selectedRow.kind === 'Pod' ? h('button', { className: classNames('tab-button', drawerTab === 'env' && 'active'), onClick: () => setDrawerTab('env') }, 'Env') : null,
        selectedRow.kind === 'Pod' ? h('button', { className: classNames('tab-button', drawerTab === 'logs' && 'active'), onClick: () => setDrawerTab('logs') }, 'Logs') : null,
        selectedRow.kind === 'Pod' ? h('button', { className: classNames('tab-button', drawerTab === 'shell' && 'active'), onClick: () => setDrawerTab('shell') }, 'Shell') : null,
        detailKinds.has(selectedRow.kind) ? h('button', { className: classNames('tab-button', drawerTab === 'yaml' && 'active'), onClick: () => setDrawerTab('yaml') }, 'YAML') : null
      ),
      selectedRow.kind === 'Pod' || selectedRow.kind === 'Deployment' || selectedRow.kind === 'StatefulSet' || selectedRow.kind === 'DaemonSet' || selectedRow.kind === 'Namespace'
        ? h('div', { className: 'actions-row drawer-actions' },
            selectedRow.kind === 'Pod' ? h('button', { className: 'button ghost', onClick: confirmDelete }, 'Delete pod') : null,
            selectedRow.kind === 'Pod' ? h('button', { className: 'button ghost', onClick: () => runAction('portforward', { localPort: 8080, remotePort: 8080 }) }, 'Port-forward :8080') : null,
            selectedRow.kind === 'Pod' ? h('button', { className: 'button ghost', onClick: () => setDrawerTab('shell') }, 'Open shell') : null,
            ['Deployment', 'StatefulSet', 'DaemonSet'].includes(selectedRow.kind) ? h('button', { className: 'button ghost', onClick: confirmRestart }, 'Restart') : null,
            ['Deployment', 'StatefulSet'].includes(selectedRow.kind) ? h('button', { className: 'button ghost', onClick: () => {
              const match = String(selectedRow.status || '').match(/(\d+)\//);
              const current = match ? Number(match[1]) : 1;
              runAction('scale', { replicas: current + 1 });
            } }, 'Scale +1') : null,
            ['Deployment', 'StatefulSet'].includes(selectedRow.kind) ? h('button', { className: 'button ghost', onClick: () => {
              const match = String(selectedRow.status || '').match(/(\d+)\//);
              const current = match ? Number(match[1]) : 1;
              runAction('scale', { replicas: Math.max(0, current - 1) });
            } }, 'Scale -1') : null,
            selectedRow.kind === 'Deployment' || selectedRow.kind === 'Namespace'
              ? h('button', { className: 'button ghost', onClick: confirmDelete }, selectedRow.kind === 'Namespace' ? 'Delete namespace' : 'Delete')
              : null
          )
        : null,
      h('div', { className: 'drawer-body' },
        drawerTab === 'overview'
          ? h(ResourceOverview, {
              row: selectedRow,
              yamlContent: null,
              yamlLoading: false
            })
          : null,
        drawerTab === 'yaml'
          ? h(YamlEditor, {
              value: drawerDetail?.content || '',
              loading: drawerLoading,
              editable: editableYamlKinds.has(selectedRow.kind),
              dark: effectiveTheme === 'dark',
              onStatus: (message) => setStatus(message),
              onApply: async () => {
                await loadDrawerOverview(selectedRow);
                if (activeItem) await loadItemRows(activeItem);
              }
            })
          : null,
        drawerTab === 'containers' ? h(ContainersPanel, { row: selectedRow }) : null,
        drawerTab === 'env' ? h(EnvPanel, { content: envContent, loading: drawerLoading }) : null,
        drawerTab === 'shell' && selectedRow.kind === 'Pod'
          ? h(React.Fragment, null,
              h('div', { className: 'toolbar-grid drawer-log-toolbar' },
                h(ContainerPicker, {
                  containers: containerOptions,
                  value: selectedContainer,
                  onChange: setSelectedContainer,
                  required: true,
                  label: 'Shell container'
                })
              ),
              h(ShellTerminal, {
                namespace: selectedRow.namespace,
                pod: selectedRow.name,
                container: selectedContainer || ''
              })
            )
          : null,
        drawerTab === 'logs' ? h(React.Fragment, null,
          h('div', { className: 'toolbar-grid drawer-log-toolbar' },
            h(ContainerPicker, {
              containers: containerOptions,
              value: selectedContainer,
              onChange: setSelectedContainer,
              required: true,
              label: 'Log container'
            }),
            h('label', { className: 'toolbar-field' },
              h('span', null, 'Level'),
              h('select', { value: logLevel, onChange: (event) => setLogLevel(event.target.value) },
                ...levelFilters.map((level) => h('option', { key: level, value: level }, level))
              )
            ),
            h('label', { className: 'toolbar-field search-grow' },
              h('span', null, 'Search'),
              h('input', { value: logSearch, onChange: (event) => setLogSearch(event.target.value), placeholder: 'Substring search…' })
            ),
            h('div', { className: 'live-indicator' },
              h('span', { className: classNames('pulse-dot', !logFollow && 'paused') }),
              h('button', { className: 'button ghost', onClick: () => setLogFollow((current) => !current) }, logFollow ? 'Live on' : 'Live off')
            )
          ),
          !selectedContainer
            ? h('div', { className: 'empty-panel' },
                h('h3', null, 'Select a container'),
                h('p', null, containerOptions.length > 1
                  ? 'This pod has multiple containers. Choose one to stream logs.'
                  : 'Waiting for container list…')
              )
            : h('div', { className: 'log-card drawer-log-card' },
            h('div', { className: 'log-header' }, h('span', null, 'Timestamp'), h('span', null, 'Level'), h('span', null, 'Message')),
            h('div', { className: 'log-body', ref: logBodyRef, onScroll: handleLogScroll },
              logsLoading && !filteredLogs.length
                ? h('div', { className: 'empty-panel' }, 'Loading logs…')
                : (filteredLogs.length
                  ? filteredLogs.map((entry, index) => h('div', { key: `${index}-${entry.raw}`, className: 'log-row' },
                      h('span', { className: 'log-time' }, entry.timestamp),
                      h('span', { className: classNames('log-level', entry.level.toLowerCase()) }, compactLevel(entry.level)),
                      h('span', { className: 'log-message' }, entry.message)
                    ))
                  : h('div', { className: 'empty-panel' }, 'No logs to show'))
            ),
            h('div', { className: 'log-footer' },
              h('button', { className: 'button ghost', onClick: () => refreshPodLogs(selectedRow) }, 'Refresh'),
              h('button', { className: 'button ghost', onClick: () => copyText(filteredLogs.map((entry) => entry.raw).join('\n')) }, 'Copy logs')
            )
          )
        ) : null
      )
    )
  ) : null;

  const confirmModal = confirmState ? h('div', { className: 'confirm-backdrop' },
    h('div', { className: 'confirm-modal' },
      h('h3', null, confirmState.title),
      h('p', null, confirmState.message),
      h('div', { className: 'confirm-actions' },
        h('button', { className: 'button ghost', onClick: () => setConfirmState(null) }, 'Cancel'),
        h('button', { className: 'button primary danger', onClick: handleConfirm }, confirmState.confirmLabel)
      )
    )
  ) : null;

  return h('div', { className: classNames('viewer-shell', sidebarCollapsed && 'sidebar-collapsed') },
    h('div', { className: classNames('sidebar-scrim', sidebarOpen && 'visible'), onClick: () => setSidebarOpen(false) }),
    sidebar,
    h('main', { className: 'main-panel' },
      topbar,
      feedback,
      pageContent
    ),
    drawer,
    confirmModal
  );
}

createRoot(document.getElementById('root')).render(h(App));
