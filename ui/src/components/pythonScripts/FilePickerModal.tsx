import { useState } from 'react'
import { Folder, FileCode2, ChevronUp, Loader2, AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { useBrowsePythonDir } from '@/hooks/usePythonScripts'
import { cn } from '@/lib/utils'
import type { BrowseEntry } from '@/types/pythonScript'

interface FilePickerRowProps {
  entry: BrowseEntry
  selected: boolean
  onDirClick: () => void
  onFileClick: () => void
}

function FilePickerRow({ entry, selected, onDirClick, onFileClick }: FilePickerRowProps) {
  if (entry.is_dir) {
    return (
      <button
        type="button"
        onClick={onDirClick}
        className="flex items-center gap-2 w-full text-left px-3 py-2 rounded hover:bg-muted text-sm"
      >
        <Folder className="h-4 w-4 shrink-0 text-blue-500" />
        <span>{entry.name}</span>
      </button>
    )
  }

  return (
    <button
      type="button"
      onClick={onFileClick}
      className={cn(
        'flex items-center gap-2 w-full text-left px-3 py-2 rounded text-sm',
        selected ? 'bg-primary/10 text-primary' : 'hover:bg-muted'
      )}
    >
      <FileCode2 className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span>{entry.name}</span>
    </button>
  )
}

interface FilePickerModalProps {
  open: boolean
  onClose: () => void
  onSelect: (path: string) => void
}

export function FilePickerModal({ open, onClose, onSelect }: FilePickerModalProps) {
  const [currentPath, setCurrentPath] = useState<string | undefined>(undefined)
  const [selectedFile, setSelectedFile] = useState<string | null>(null)

  const { data, isLoading, isError, error } = useBrowsePythonDir(currentPath)

  const navigateToChild = (name: string) => {
    const base = data?.path ?? ''
    const childPath = base.endsWith('/') ? base + name : base + '/' + name
    setCurrentPath(childPath)
    setSelectedFile(null)
  }

  const navigateUp = () => {
    const parts = (data?.path ?? '').split('/')
    const parent = parts.slice(0, -1).join('/') || '/'
    setCurrentPath(parent)
    setSelectedFile(null)
  }

  const atRoot = !data?.path || data.path === '/'

  const handleClose = () => {
    setCurrentPath(undefined)
    setSelectedFile(null)
    onClose()
  }

  const handleSelect = () => {
    if (selectedFile) onSelect(selectedFile)
  }

  const dirs = data?.entries.filter((e) => e.is_dir) ?? []
  const pyFiles = data?.entries.filter((e) => !e.is_dir && e.is_python) ?? []

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-xl">
      <DialogHeader onClose={handleClose}>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={navigateUp}
            disabled={atRoot}
            title="Go to parent directory"
          >
            <ChevronUp className="h-4 w-4" />
          </Button>
          <span className="text-sm font-mono truncate text-muted-foreground">
            {data?.path ?? '…'}
          </span>
        </div>
      </DialogHeader>
      <DialogBody>
        {isLoading && (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
          </div>
        )}
        {isError && (
          <div className="flex items-center gap-2 text-destructive py-4">
            <AlertCircle className="h-4 w-4 shrink-0" />
            <span className="text-sm">{(error as Error)?.message ?? 'Failed to load directory'}</span>
          </div>
        )}
        {!isLoading && !isError && (
          <div className="space-y-0.5">
            {dirs.length === 0 && pyFiles.length === 0 && (
              <p className="text-sm text-muted-foreground text-center py-4">No files or directories</p>
            )}
            {dirs.map((entry) => (
              <FilePickerRow
                key={entry.name}
                entry={entry}
                selected={false}
                onDirClick={() => navigateToChild(entry.name)}
                onFileClick={() => {}}
              />
            ))}
            {pyFiles.map((entry) => {
              const base = data?.path ?? ''
              const absPath = base.endsWith('/') ? base + entry.name : base + '/' + entry.name
              return (
                <FilePickerRow
                  key={entry.name}
                  entry={entry}
                  selected={selectedFile === absPath}
                  onDirClick={() => {}}
                  onFileClick={() => setSelectedFile(absPath)}
                />
              )
            })}
          </div>
        )}
      </DialogBody>
      <DialogFooter>
        <Button type="button" variant="ghost" size="sm" onClick={handleClose}>
          Cancel
        </Button>
        <Button type="button" size="sm" disabled={!selectedFile} onClick={handleSelect}>
          Select
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
