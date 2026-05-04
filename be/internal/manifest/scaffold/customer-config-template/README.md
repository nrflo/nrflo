# {{Name}} Customer Config

This directory contains the tool manifest and scripts for the **{{Name}}** customer configuration.

## Structure

```
tool_manifest.yaml      # Declares all tools and their schemas
tools/
  lookup_sku.py         # Python script: look up product details by SKU
```

## How It Works

Each tool in `tool_manifest.yaml` has `type: python_script`. The script receives
a JSON object on stdin (validated against `input_schema`) and must write a JSON
object to stdout. A non-zero exit code signals an error.

### Adding a New Tool

1. Create a Python script under `tools/`.
2. Add an entry to `tool_manifest.yaml`:
   ```yaml
   - name: my_tool
     type: python_script
     description: What this tool does
     script: tools/my_tool.py
     input_schema:
       type: object
       properties:
         param:
           type: string
       required:
         - param
   ```
3. The manifest is validated on load — only `python_script` is supported.

## Security Notes

- Scripts run with a restricted environment (only explicitly allowed env vars pass through).
- Execution is subject to a per-invocation timeout (default 5 seconds).
- Script paths must be relative and must not traverse parent directories.
