## Unity
- Every asset has a `.meta` file: move, rename or delete the two together, never edit or regenerate a GUID.
- Scenes, prefabs and uGUI change only through the Unity MCP server. If none is connected, say so and list the Editor steps.
- Before adding a reference between folders, check which `.asmdef` owns each one. Editor-only code goes in `Editor/` or `#if UNITY_EDITOR`.
- No allocations or repeated `GetComponent`/`Find` in `Update` or other per-frame code.
- Batch-mode tests can't open a project already open in the Editor: report it and ask to close the Editor; don't retry.
