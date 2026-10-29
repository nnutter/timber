package timber

const herdrPluginDirectoryName = "timber"

// HerdrKeybindingTOML documents the Herdr keybindings for the Timber plugin.
const HerdrKeybindingTOML = `[[keys.command]]
key = "prefix+shift+s"
type = "plugin_action"
command = "nnutter.timber.open"
description = "open or create Timber Space"

[[keys.command]]
key = "prefix+shift+d"
type = "plugin_action"
command = "nnutter.timber.delete"
description = "delete Timber worktree"
`
