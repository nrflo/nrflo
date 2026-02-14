import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { DiffViewer } from './DiffViewer'

const sampleDiff = `diff --git a/src/feature.ts b/src/feature.ts
new file mode 100644
index 0000000..abc123
--- /dev/null
+++ b/src/feature.ts
@@ -0,0 +1,5 @@
+export function newFeature() {
+  const value = "feature"
+  return value
+}
+`

const multiFileDiff = `diff --git a/src/file1.ts b/src/file1.ts
index abc123..def456 100644
--- a/src/file1.ts
+++ b/src/file1.ts
@@ -1,3 +1,4 @@
 export function file1() {
+  console.log("added line")
   return "file1"
 }
diff --git a/src/file2.ts b/src/file2.ts
index def456..ghi789 100644
--- a/src/file2.ts
+++ b/src/file2.ts
@@ -1,4 +1,3 @@
 export function file2() {
-  console.log("removed line")
   return "file2"
 }`

const complexDiff = `diff --git a/src/main.ts b/src/main.ts
index abc123..def456 100644
--- a/src/main.ts
+++ b/src/main.ts
@@ -10,7 +10,7 @@ function main() {
   const config = loadConfig()

   if (config.enabled) {
-    console.log("Old message")
+    console.log("New message")
   }

   return config
@@ -25,3 +25,5 @@ function helper() {
   return data
 }
+
+export { main, helper }`

describe('DiffViewer - Basic Rendering', () => {
  it('renders single file diff with filename', () => {
    render(<DiffViewer diff={sampleDiff} />)

    expect(screen.getByText('src/feature.ts')).toBeInTheDocument()
  })

  it('renders added lines with green background', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />)

    const addedLines = container.querySelectorAll('.bg-green-50')
    expect(addedLines.length).toBeGreaterThan(0)
  })

  it('renders hunk header lines', () => {
    render(<DiffViewer diff={sampleDiff} />)

    expect(screen.getByText('@@ -0,0 +1,5 @@')).toBeInTheDocument()
  })

  it('renders "No diff available" when diff is empty', () => {
    render(<DiffViewer diff="" />)

    expect(screen.getByText('No diff available')).toBeInTheDocument()
  })

  it('renders "No diff available" when diff is whitespace only', () => {
    render(<DiffViewer diff="   \n  " />)

    expect(screen.getByText('No diff available')).toBeInTheDocument()
  })
})

describe('DiffViewer - Multi-File Diffs', () => {
  it('renders multiple file sections', () => {
    render(<DiffViewer diff={multiFileDiff} />)

    expect(screen.getByText('src/file1.ts')).toBeInTheDocument()
    expect(screen.getByText('src/file2.ts')).toBeInTheDocument()
  })

  it('renders added lines in first file', () => {
    const { container } = render(<DiffViewer diff={multiFileDiff} />)

    const addedLines = container.querySelectorAll('.bg-green-50')
    expect(addedLines.length).toBeGreaterThan(0)
    // Check that the line content contains the added text
    const hasAddedLine = Array.from(addedLines).some(el => el.textContent?.includes('console.log("added line")'))
    expect(hasAddedLine).toBe(true)
  })

  it('renders removed lines in second file', () => {
    const { container } = render(<DiffViewer diff={multiFileDiff} />)

    const removedLines = container.querySelectorAll('.bg-red-50')
    expect(removedLines.length).toBeGreaterThan(0)
    // Check that the line content contains the removed text
    const hasRemovedLine = Array.from(removedLines).some(el => el.textContent?.includes('console.log("removed line")'))
    expect(hasRemovedLine).toBe(true)
  })

  it('separates files into distinct sections', () => {
    const { container } = render(<DiffViewer diff={multiFileDiff} />)

    // Each file should be in its own bordered container
    const fileSections = container.querySelectorAll('[id^="diff-"]')
    expect(fileSections.length).toBe(2)
  })
})

describe('DiffViewer - Line Type Styling', () => {
  it('applies green background to added lines', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />)

    const addedLines = Array.from(container.querySelectorAll('.bg-green-50'))
    expect(addedLines.length).toBeGreaterThan(0)

    // Check dark mode class is also present
    const darkModeAddedLines = Array.from(container.querySelectorAll('.dark\\:bg-green-950\\/40'))
    expect(darkModeAddedLines.length).toBeGreaterThan(0)
  })

  it('applies red background to removed lines', () => {
    const diffWithDeletion = `diff --git a/test.ts b/test.ts
index abc..def 100644
--- a/test.ts
+++ b/test.ts
@@ -1,2 +1 @@
-removed line
 kept line`

    const { container } = render(<DiffViewer diff={diffWithDeletion} />)

    const removedLines = Array.from(container.querySelectorAll('.bg-red-50'))
    expect(removedLines.length).toBeGreaterThan(0)
  })

  it('applies blue background to hunk headers', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />)

    const hunkHeaders = Array.from(container.querySelectorAll('.bg-blue-50'))
    expect(hunkHeaders.length).toBeGreaterThan(0)
  })

  it('does not apply special styling to context lines', () => {
    const diffWithContext = `diff --git a/test.ts b/test.ts
index abc..def 100644
--- a/test.ts
+++ b/test.ts
@@ -1,3 +1,3 @@
 context line 1
-old line
+new line
 context line 2`

    render(<DiffViewer diff={diffWithContext} />)

    expect(screen.getByText('context line 1')).toBeInTheDocument()
    expect(screen.getByText('context line 2')).toBeInTheDocument()
  })
})

describe('DiffViewer - Diff Parsing', () => {
  it('extracts filename from diff header', () => {
    render(<DiffViewer diff={sampleDiff} />)

    // Should extract "src/feature.ts" from "a/src/feature.ts b/src/feature.ts"
    expect(screen.getByText('src/feature.ts')).toBeInTheDocument()
  })

  it('handles filename with spaces', () => {
    const diffWithSpaces = `diff --git a/src/my file.ts b/src/my file.ts
new file mode 100644
--- /dev/null
+++ b/src/my file.ts
@@ -0,0 +1 @@
+content`

    render(<DiffViewer diff={diffWithSpaces} />)

    expect(screen.getByText('src/my file.ts')).toBeInTheDocument()
  })

  it('only renders lines after @@ markers', () => {
    render(<DiffViewer diff={sampleDiff} />)

    // These header lines should not be rendered as diff content
    expect(screen.queryByText('new file mode 100644')).not.toBeInTheDocument()
    expect(screen.queryByText('index 0000000..abc123')).not.toBeInTheDocument()
    expect(screen.queryByText('--- /dev/null')).not.toBeInTheDocument()
  })

  it('handles multiple hunks in single file', () => {
    const { container } = render(<DiffViewer diff={complexDiff} />)

    // Should have two @@ markers (two hunks) - check by CSS class
    const hunkHeaders = container.querySelectorAll('.bg-blue-50')
    expect(hunkHeaders.length).toBe(2)
  })
})

describe('DiffViewer - File Section IDs', () => {
  it('generates correct ID for file section', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />)

    const fileSection = container.querySelector('#diff-src\\/feature\\.ts')
    expect(fileSection).toBeInTheDocument()
  })

  it('generates unique IDs for multiple files', () => {
    const { container } = render(<DiffViewer diff={multiFileDiff} />)

    expect(container.querySelector('#diff-src\\/file1\\.ts')).toBeInTheDocument()
    expect(container.querySelector('#diff-src\\/file2\\.ts')).toBeInTheDocument()
  })
})

describe('DiffViewer - Edge Cases', () => {
  it('handles diff with only context lines', () => {
    const contextOnlyDiff = `diff --git a/test.ts b/test.ts
index abc..def 100644
--- a/test.ts
+++ b/test.ts
@@ -1,3 +1,3 @@
 line 1
 line 2
 line 3`

    render(<DiffViewer diff={contextOnlyDiff} />)

    expect(screen.getByText('line 1')).toBeInTheDocument()
    expect(screen.getByText('line 2')).toBeInTheDocument()
    expect(screen.getByText('line 3')).toBeInTheDocument()
  })

  it('handles binary file diff gracefully', () => {
    const binaryDiff = `diff --git a/image.png b/image.png
index abc123..def456 100644
Binary files a/image.png and b/image.png differ`

    const { container } = render(<DiffViewer diff={binaryDiff} />)

    // Should parse but have no hunk content to display
    const fileSections = container.querySelectorAll('[id^="diff-"]')
    // Binary diffs have no @@ markers, so no sections are created
    expect(fileSections.length).toBe(0)
  })

  it('handles malformed diff gracefully', () => {
    const malformedDiff = 'this is not a valid diff'

    render(<DiffViewer diff={malformedDiff} />)

    // Should show "No diff available" since no valid sections found
    expect(screen.getByText('No diff available')).toBeInTheDocument()
  })

  it('handles diff starting with diff --git (no leading content)', () => {
    const cleanDiff = `diff --git a/file.ts b/file.ts
index abc..def 100644
--- a/file.ts
+++ b/file.ts
@@ -1 +1 @@
-old
+new`

    render(<DiffViewer diff={cleanDiff} />)

    expect(screen.getByText('file.ts')).toBeInTheDocument()
    expect(screen.getByText('-old')).toBeInTheDocument()
    expect(screen.getByText('+new')).toBeInTheDocument()
  })

  it('preserves whitespace in diff lines', () => {
    const diffWithWhitespace = `diff --git a/test.ts b/test.ts
index abc..def 100644
--- a/test.ts
+++ b/test.ts
@@ -1 +1 @@
-  indented old
+    more indented new`

    const { container } = render(<DiffViewer diff={diffWithWhitespace} />)

    // Check that whitespace-pre class is applied and content includes the text
    const diffLines = container.querySelectorAll('.whitespace-pre')
    expect(diffLines.length).toBeGreaterThan(0)
    const hasIndentedContent = Array.from(diffLines).some(el =>
      el.textContent?.includes('indented old') || el.textContent?.includes('more indented new')
    )
    expect(hasIndentedContent).toBe(true)
  })

  it('handles very long lines without breaking layout', () => {
    const longLineDiff = `diff --git a/test.ts b/test.ts
index abc..def 100644
--- a/test.ts
+++ b/test.ts
@@ -1 +1 @@
-${'x'.repeat(500)}
+${'y'.repeat(500)}`

    const { container } = render(<DiffViewer diff={longLineDiff} />)

    // Should render with overflow-x-auto
    const scrollContainer = container.querySelector('.overflow-x-auto')
    expect(scrollContainer).toBeInTheDocument()
  })

  it('limits max height per file section', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />)

    // Each file diff container should have max-h-[600px]
    const diffContainer = container.querySelector('.max-h-\\[600px\\]')
    expect(diffContainer).toBeInTheDocument()
  })
})

describe('DiffViewer - Monospace Rendering', () => {
  it('renders diff lines in monospace font', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />)

    const diffLines = container.querySelectorAll('.font-mono')
    expect(diffLines.length).toBeGreaterThan(0)
  })

  it('renders filename in monospace font', () => {
    const { container } = render(<DiffViewer diff={sampleDiff} />)

    const filename = screen.getByText('src/feature.ts')
    expect(filename).toHaveClass('font-mono')
  })
})
