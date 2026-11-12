## Unity
- Never read or search `Library/`, `Temp/`, `Logs/`, `obj/`, `UserSettings/`, or `Packages/packages-lock.json`.
- Scenes, prefabs and assets (`*.unity`, `*.prefab`, `*.asset`, `*.mat`) are large YAML: open them only when the task is about them, and search for the GUID or field instead of reading the file.
- Every asset has a `.meta` file. Move, rename or delete the two together, and never edit or regenerate a GUID: that breaks every reference to the asset.
- Change uGUI, scenes and prefabs (`*.unity`, `*.prefab`) through the Unity MCP server, never by editing their YAML. If no Unity MCP server is connected, say so and list the exact Editor steps instead.
- Before adding a reference between folders, check which `.asmdef` owns each one. Editor-only code goes in an `Editor/` folder or inside `#if UNITY_EDITOR`.
- Change `ProjectSettings/` and `Packages/manifest.json` only when the task asks for it.
- Avoid allocations and repeated `GetComponent`/`Find` calls in `Update` and other per-frame code.
- Batch-mode tests can't open a project that is already open in the Editor. If they fail for that reason, report it and ask to close the Editor; don't retry.
