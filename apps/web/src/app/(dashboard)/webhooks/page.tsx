'use client';

import { useEffect, useState, useCallback } from 'react';
import { api, type WebhookEndpoint, type WebhookDelivery } from '@/lib/api';
import { useToast } from '@/lib/toast-context';
import { PageHeader } from '@/components/ui/page-header';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { EmptyState } from '@/components/ui/empty-state';
import { Skeleton } from '@/components/ui/skeleton';
import { VerifySignatureTool } from '@/components/webhooks/verify-signature-tool';
import { Webhook, Plus, X, Trash2 } from 'lucide-react';

const eventOptions = [
  'transfer.initiated',
  'transfer.settled',
  'transfer.failed',
  'wallet.funded',
  'conversion.completed',
];

function deliveryStatusBadge(status: string) {
  if (status === 'success') return <Badge variant="success">{status}</Badge>;
  return <Badge variant="danger">{status}</Badge>;
}

export default function WebhooksPage() {
  const { toast } = useToast();
  const [endpoints, setEndpoints] = useState<WebhookEndpoint[]>([]);
  const [deliveries, setDeliveries] = useState<WebhookDelivery[]>([]);
  const [loading, setLoading] = useState(true);
  const [url, setUrl] = useState('');
  const [selectedEvents, setSelectedEvents] = useState<string[]>([
    'transfer.settled',
    'transfer.failed',
    'wallet.funded',
  ]);
  const [registering, setRegistering] = useState(false);
  const [showForm, setShowForm] = useState(false);

  const fetchEndpoints = useCallback(async () => {
    setLoading(true);
    try {
      const res = await api.listWebhooks();
      setEndpoints(res.endpoints || []);
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to load webhooks', 'error');
    } finally {
      setLoading(false);
    }
  }, [toast]);

  useEffect(() => {
    let cancelled = false;
    const run = async () => {
      if (cancelled) return;
      await fetchEndpoints();
    };
    run();
    return () => {
      cancelled = true;
    };
  }, [fetchEndpoints]);

  useEffect(() => {
    if (endpoints.length === 0) return;
    let cancelled = false;
    const loadDeliveries = async () => {
      const all: WebhookDelivery[] = [];
      for (const ep of endpoints) {
        try {
          const res = await api.listDeliveries(ep.id, 10);
          all.push(...(res.deliveries || []));
        } catch {}
      }
      if (!cancelled) {
        setDeliveries(all);
      }
    };
    loadDeliveries();
    return () => {
      cancelled = true;
    };
  }, [endpoints]);

  const handleRegister = async (e: React.FormEvent) => {
    e.preventDefault();
    setRegistering(true);
    try {
      await api.registerWebhook(url, selectedEvents);
      toast('Webhook registered', 'success');
      setShowForm(false);
      setUrl('');
      await fetchEndpoints();
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to register webhook', 'error');
    } finally {
      setRegistering(false);
    }
  };

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this webhook endpoint?')) return;
    try {
      await api.deleteWebhook(id);
      toast('Webhook deleted', 'success');
      await fetchEndpoints();
    } catch (err) {
      toast(err instanceof Error ? err.message : 'Failed to delete webhook', 'error');
    }
  };

  const toggleEvent = (event: string) => {
    setSelectedEvents((prev) =>
      prev.includes(event) ? prev.filter((e) => e !== event) : [...prev, event]
    );
  };

  if (loading) {
    return (
      <div className="flex flex-col gap-8">
        <div className="flex items-center justify-between">
          <Skeleton className="h-10 w-48" />
          <Skeleton className="h-10 w-36" />
        </div>
        <Skeleton className="h-64" />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8 animate-in fade-in slide-in-from-bottom-4 duration-500">
      <PageHeader
        title="Webhooks"
        description="Configure webhooks to receive real-time event notifications."
      >
        <Button
          variant={showForm ? 'secondary' : 'primary'}
          onClick={() => setShowForm(!showForm)}
        >
          {showForm ? (
            <>
              <X className="h-4 w-4" /> Cancel
            </>
          ) : (
            <>
              <Plus className="h-4 w-4" /> Add Endpoint
            </>
          )}
        </Button>
      </PageHeader>

      {showForm && (
        <Card className="max-w-2xl">
          <CardHeader>
            <CardTitle>Register Webhook Endpoint</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={handleRegister} className="flex flex-col gap-5">
              <div className="flex flex-col gap-1.5">
                <label className="text-sm font-medium text-foreground">Webhook URL</label>
                <Input
                  type="url"
                  value={url}
                  onChange={(e) => setUrl(e.target.value)}
                  required
                  placeholder="https://your-domain.com/webhook"
                />
              </div>
              <div className="flex flex-col gap-2">
                <label className="text-sm font-medium text-foreground">Events to send</label>
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                  {eventOptions.map((event) => (
                    <label
                      key={event}
                      className="flex items-center gap-3 text-sm text-foreground cursor-pointer"
                    >
                      <input
                        type="checkbox"
                        checked={selectedEvents.includes(event)}
                        onChange={() => toggleEvent(event)}
                        className="h-4 w-4 rounded border-border text-primary accent-primary"
                      />
                      {event}
                    </label>
                  ))}
                </div>
              </div>
              <div className="flex justify-end">
                <Button type="submit" isLoading={registering} disabled={!url}>
                  Register Endpoint
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {endpoints.length > 0 && (
        <div className="flex flex-col gap-4">
          <h2 className="text-lg font-semibold">Registered Endpoints</h2>
          <div className="grid grid-cols-1 gap-4">
            {endpoints.map((ep) => (
              <Card key={ep.id}>
                <CardContent className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between">
                  <div className="flex flex-col gap-1 min-w-0">
                    <code className="truncate font-mono text-sm text-foreground">
                      {ep.url}
                    </code>
                    <span className="text-xs text-muted-foreground">
                      Events:{' '}
                      {ep.events.length > 0 ? ep.events.join(', ') : 'All'}
                    </span>
                  </div>
                  <div className="flex items-center gap-3">
                    <Badge variant={ep.active ? 'success' : 'default'}>
                      {ep.active ? 'Active' : 'Inactive'}
                    </Badge>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="text-danger hover:bg-danger-subtle hover:text-danger"
                      onClick={() => handleDelete(ep.id)}
                    >
                      <Trash2 className="h-4 w-4" />
                      Delete
                    </Button>
                  </div>
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      )}

      <div className="flex flex-col gap-4">
        <h2 className="text-lg font-semibold">Recent Delivery Logs</h2>
        <Card>
          {deliveries.length === 0 ? (
            <EmptyState
              icon={Webhook}
              title="No delivery logs yet"
              description="Webhook deliveries will appear here once events are sent."
            />
          ) : (
            <Table>
              <TableHead>
                <TableRow>
                  <TableHeader>Event Type</TableHeader>
                  <TableHeader>Endpoint</TableHeader>
                  <TableHeader>Status</TableHeader>
                  <TableHeader>Response</TableHeader>
                  <TableHeader>Time</TableHeader>
                </TableRow>
              </TableHead>
              <TableBody>
                {deliveries.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell className="font-medium">{log.event_type}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">
                      {log.endpoint_id.slice(0, 8)}...
                    </TableCell>
                    <TableCell>{deliveryStatusBadge(log.status)}</TableCell>
                    <TableCell className="text-muted-foreground">
                      {log.response_code || '—'}
                    </TableCell>
                    <TableCell className="text-muted-foreground">
                      {new Date(log.created_at).toLocaleString()}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </Card>
      </div>

      <div className="flex flex-col gap-4">
        <h2 className="text-lg font-semibold">Verify Signature</h2>
        <VerifySignatureTool />
      </div>
    </div>
  );
}
