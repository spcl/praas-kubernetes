const delimChars = [" ", "\"", "-", "."]
const maxVisibleEntries = 1000

function isBlank(str) {
    return (!str || /^\s*$/.test(str));
}

function getScrollArea() {
    return document.getElementById("log-pane")
}

const app = Vue.createApp({
    data() {
        return {
            scrollAreaHeight: "500px",
            filterText: "",
            prevFilterText: "",
            wasAtBottom: true,
            logs: [],
            maxLineLength: 107,
            isCollecting: true
        }
    },
    setup() {
        Vue.onUpdated(() => {
            // Scroll to the bottom if needed
            let scrollArea = getScrollArea()
            if (app.wasAtBottom) {
                scrollArea.scrollTop = scrollArea.scrollHeight;
            }
        })
        Vue.onBeforeUpdate(() => {
            // Should we scroll to the bottom after the update?
            let scrollArea = getScrollArea()
            app.wasAtBottom = scrollArea.scrollHeight - scrollArea.scrollTop - scrollArea.clientHeight < 5
        })
    },
    mounted() {
        window.addEventListener("resize", this.handleWindowResize);
        this.handleWindowResize(null)
    },
    computed: {
        filteredLogs() {
            let context = this
            let filteredLogs

            // Filter according to the filter criteria
            if (!isBlank(context.filterText)) {
                let split_text = context.filterText.split(":")
                let field = "message"
                let text = context.filterText
                if (split_text.length > 1) {
                    field = split_text[0]
                    text = split_text.splice(1).join(":")
                }
                filteredLogs = context.logs.filter(entry => field in entry && entry[field].toLowerCase().includes(text.toLowerCase()))
            } else {
                filteredLogs = context.logs
            }

            // let numItems = filteredLogs.length
            // filteredLogs = filteredLogs.slice(numItems - maxVisibleEntries - 1, numItems - 1)

            // Check if we need to resize any entries
            filteredLogs.forEach((log_entry, index, arr) => {
                if (log_entry.orig_msg.length > context.maxLineLength) {
                    // Find indices of all spaces
                    let space_indices = []
                    for (let i = log_entry.orig_msg.length; i--;) {
                        if (delimChars.includes(log_entry.orig_msg[i])) space_indices.push(i);
                    }
                    // Find first space smaller than max length
                    let lineLen = -1
                    for (let i = 0; i < space_indices.length && lineLen === -1; i++) {
                        if (space_indices[i] <= context.maxLineLength)
                            lineLen = space_indices[i]
                    }
                    log_entry.extra = log_entry.orig_msg.substring(lineLen+1)
                    log_entry.message = log_entry.orig_msg.substring(0, lineLen+1)
                }
                else {
                    log_entry.message = log_entry.orig_msg
                }
                arr[index] = log_entry
            })

            return filteredLogs
        },
        instances() {
            return [...new Set(this.logs.map(log=>log.instance))]
        },
    },
    methods: {
        addLogEntry(log_entry) {
            if (app.isCollecting) {
                log_entry.orig_msg = log_entry.message
                app.logs.push(log_entry)
            }
        },
        handleWindowResize(e) {
            let height = document.documentElement.clientHeight;
            let logPane = document.getElementById("log-pane")
            let bounding = logPane.getBoundingClientRect();
            this.scrollAreaHeight = height - bounding.top + "px";

            let logAreaWidth = logPane.clientWidth

            // So this is kinda hacky, but:
            //  - Generally speaking one character needs 9 or 10 pixels
            //  - Additionally there is ~250-270 pixels we can't use
            //  - So usually for x characters we need = [250 + 9x, 270 + 10x]
            this.maxLineLength = Math.floor((logAreaWidth - 260) / 10 - 1) //logAreaWidth * 1.3 / 16
        },
        switchCollection() {
            this.isCollecting = !this.isCollecting
        },
        clear() {
            this.logs = []
        },
        setFilter(instanceName) {
            if (this.isFilterActive(instanceName)) {
                this.filterText = this.prevFilterText
            } else {
                this.prevFilterText = this.filterText
                this.filterText = "instance:" + instanceName
            }
        },
        isFilterActive(instanceName) {
            return this.filterText === "instance:" + instanceName
        },
    }
}).mount("#app")

const source = new EventSource("/log-stream");
source.addEventListener("logAddition", function(event) {
    if (event.data.startsWith("{")) {
        let data = JSON.parse(event.data);
        app.$options.methods.addLogEntry(data)
    } else {
        console.log(event.data)
    }
}, false);

source.addEventListener("error", function(event) {
    console.log(event)
    // alert("Failed to connect to event stream. Is Redis running?");
}, false);
