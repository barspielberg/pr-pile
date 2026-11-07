---
title: Keys
description: Every key binding, split by the context it works in.
---

pile has four contexts, each with its own keys: the board, the search prompt,
the detail overlay, and the help page. Actions you configure add their own keys
to the board and the help page.

## Board

| key | action |
| --- | --- |
| `j` / `k` | move down / up (also `down` / `up`) |
| `ctrl+d` / `ctrl+u` | half a page down / up |
| `pgdn` / `pgup` | a full page down / up |
| `l` / `h` | next / previous section (also `right` / `left`) |
| `g` / `G` | top / bottom (also `home` / `end`) |
| `enter` / `o` | open the selected PRs in the browser; three or more asks first |
| `S` | open the GitHub stack page, once per selected stack |
| `d` | open the detail overlay for the selected PR |
| `space` | select or deselect the PR under the cursor |
| `v` | select a range; press again to leave range mode |
| `y` | copy selected PR URLs, one per line |
| `Y` | copy menu: number, title, URL, branch, author, Markdown or stack link |
| `m` | watch the selected PRs; press again to stop watching |
| `/` | open the search prompt |
| `n` / `N` | next / previous match, wrapping |
| `r` | refresh the board |
| `?` | open the help page |
| `q` / `esc` / `ctrl+c` | quit |

`esc` first clears a selection, then a live search, then quits. `q` asks before
quitting while watches or actions are running; `ctrl+c` quits immediately.
Three or more opens or multi actions ask first: `y` or `enter` confirms, any
other key cancels and keeps the selection. Reloading clears the selection.

Watched rows show `⚐`; `⚑` means something changed. Opening the PR with
`enter` or `d` clears that unread marker. Watches end when pile quits.

## Search prompt

While the prompt is open, every printable key belongs to the query, and
navigation moves to chords.

| key | action |
| --- | --- |
| text | type into the query |
| `backspace` | delete the last character |
| `ctrl+n` / `ctrl+j` / `down` | next match while typing |
| `ctrl+p` / `ctrl+k` / `up` | previous match while typing |
| `enter` | keep the query and its highlights |
| `esc` | cancel the search |
| `ctrl+c` | quit |

## Detail overlay

The detail page scrolls and searches like help. It stays open while you read.

| key | action |
| --- | --- |
| `j` / `k` / `down` / `up` | scroll down / up |
| `ctrl+d` / `ctrl+u` | half a page down / up |
| `pgdn` / `pgup` | a full page down / up |
| `g` / `G` / `home` / `end` | top / bottom |
| `/` | search within the detail page |
| `n` / `N` | next / previous match |
| `?` | open help |
| `esc` / `q` / `d` | clear a live search or close the page |
| `ctrl+c` | quit |

## Help page

The help page scrolls and searches; the bottom row of the page always lists how
to leave it.

| key | action |
| --- | --- |
| `j` / `k` | scroll down / up (also `down` / `up`) |
| `ctrl+d` / `ctrl+u` | half a page down / up |
| `pgdn` / `pgup` | a full page down / up |
| `g` / `G` | top / bottom (also `home` / `end`) |
| `/` | search within the page |
| `n` / `N` | next / previous match |
| `esc` / `q` / `?` | close the page |
| any other key | close the page |
| `ctrl+c` | quit |

The help page's search prompt is the board's, with the cursor swapped for the
scroll position.
