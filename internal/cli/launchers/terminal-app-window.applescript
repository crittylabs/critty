on run argv
    if item 1 of argv is "close" and application id "com.apple.Terminal" is not running then return
    tell application id "com.apple.Terminal"
        if item 1 of argv is "open" then
            set reviewTab to do script ("exec " & item 2 of argv)
            set reviewTTY to tty of reviewTab
            repeat with candidate in windows
                repeat with candidateTab in tabs of candidate
                    if tty of candidateTab is reviewTTY then
                        set reviewWindowID to id of candidate
                        set index of candidate to 1
                        activate
                        return reviewWindowID as text
                    end if
                end repeat
            end repeat
            error "Cannot identify the created Terminal window"
        else if item 1 of argv is "close" then
            set reviewWindowID to (item 2 of argv) as integer
            ignoring application responses
                close (every window whose id is reviewWindowID) saving no
            end ignoring
        end if
    end tell
end run
