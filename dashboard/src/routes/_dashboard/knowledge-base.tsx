import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Text } from "@/components/ui/text";
import { Card, CardHeader, CardTitle, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { listDocuments, createDocument, deleteDocument } from "@/lib/api";
import type { Document } from "@/lib/types";
import { FileText, Plus, Trash2, X } from "lucide-react";

export const Route = createFileRoute("/_dashboard/knowledge-base")({
  component: KnowledgeBasePage,
});

function KnowledgeBasePage() {
  const queryClient = useQueryClient();
  const [showAdd, setShowAdd] = useState(false);
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [sourceUrl, setSourceUrl] = useState("");

  const { data: docs, isLoading } = useQuery({
    queryKey: ["documents"],
    queryFn: listDocuments,
  });

  const addMutation = useMutation({
    mutationFn: () => createDocument({ title, content, source_url: sourceUrl || undefined }),
    onSuccess: () => {
      setTitle("");
      setContent("");
      setSourceUrl("");
      setShowAdd(false);
      queryClient.invalidateQueries({ queryKey: ["documents"] });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: deleteDocument,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["documents"] }),
  });

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="flex items-center justify-between">
        <div>
          <Text as="h2">Knowledge Base</Text>
          <Text as="p" className="text-muted-foreground mt-1">
            Upload product docs, FAQs, and case studies. Used by AI when drafting replies.
          </Text>
        </div>
        <Button size="sm" onClick={() => setShowAdd(!showAdd)}>
          {showAdd ? <X className="h-3.5 w-3.5 mr-1.5" /> : <Plus className="h-3.5 w-3.5 mr-1.5" />}
          {showAdd ? "Cancel" : "Add Document"}
        </Button>
      </div>

      {/* Add Form */}
      {showAdd && (
        <Card>
          <CardContent className="p-4 space-y-3">
            <Input placeholder="Document title" value={title} onChange={(e) => setTitle(e.target.value)} />
            <Input placeholder="Source URL (optional)" value={sourceUrl} onChange={(e) => setSourceUrl(e.target.value)} />
            <textarea
              className="px-4 py-2 w-full rounded border-2 border-border shadow-md transition focus:outline-hidden focus:shadow-xs font-[family-name:var(--font-sans)] bg-background text-foreground placeholder:text-muted-foreground min-h-[200px] resize-y"
              placeholder="Paste your document content here (markdown supported)..."
              value={content}
              onChange={(e) => setContent(e.target.value)}
            />
            <div className="flex justify-end gap-2">
              <Button variant="outline" size="sm" onClick={() => setShowAdd(false)}>
                Cancel
              </Button>
              <Button size="sm" onClick={() => addMutation.mutate()} disabled={!title.trim() || !content.trim() || addMutation.isPending}>
                {addMutation.isPending ? "Saving..." : "Save Document"}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Loading */}
      {isLoading && (
        <Card>
          <CardContent className="p-8 text-center">
            <Text as="p" className="text-muted-foreground">Loading documents...</Text>
          </CardContent>
        </Card>
      )}

      {/* Empty */}
      {!isLoading && !(docs ?? []).length && (
        <Card>
          <CardContent className="p-8 text-center">
            <FileText className="h-8 w-8 mx-auto mb-2 text-muted-foreground" />
            <Text as="p" className="text-muted-foreground">
              No documents yet. Add product pages, FAQs, or case studies to help AI draft better replies.
            </Text>
          </CardContent>
        </Card>
      )}

      {/* Document List */}
      <div className="grid gap-3">
        {(docs ?? []).map((doc: Document) => (
          <Card key={doc.id} className="hover:shadow-sm">
            <CardHeader className="pb-2">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <FileText className="h-4 w-4 text-muted-foreground" />
                  <CardTitle className="text-base">{doc.title}</CardTitle>
                </div>
                <div className="flex items-center gap-2">
                  <Badge variant="outline" size="sm">{doc.content_type}</Badge>
                  {doc.file_size_bytes ? (
                    <Badge variant="default" size="sm">
                      {(doc.file_size_bytes / 1024).toFixed(1)} KB
                    </Badge>
                  ) : null}
                  <Button
                    variant="ghost"
                    size="icon"
                    className="h-7 w-7"
                    onClick={() => deleteMutation.mutate(doc.id)}
                    disabled={deleteMutation.isPending}
                  >
                    <Trash2 className="h-3.5 w-3.5 text-destructive" />
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent className="pt-0">
              <Text as="p" className="text-sm text-muted-foreground line-clamp-3">
                {doc.content}
              </Text>
              {doc.source_url && (
                <a href={doc.source_url} target="_blank" rel="noopener noreferrer" className="text-xs text-muted-foreground hover:text-foreground underline mt-1 inline-block">
                  {doc.source_url}
                </a>
              )}
            </CardContent>
          </Card>
        ))}
      </div>
    </div>
  );
}
