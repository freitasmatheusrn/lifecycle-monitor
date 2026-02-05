const TOAST_ICONS = {
    success: `<svg class="toast-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/>
    </svg>`,
    danger: `<svg class="toast-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
    </svg>`,
    warning: `<svg class="toast-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z"/>
    </svg>`,
    info: `<svg class="toast-icon" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/>
    </svg>`
};

const TOAST_DURATION = 5000;

class Toast {
    /**
     * A class representing a Toast notification.
     * @param level {("info"|"success"|"warning"|"danger")}
     * @param message {string}
     */
    constructor(level, message) {
        this.level = level;
        this.message = message;
        this.element = null;
        this.timeoutId = null;
    }

    /**
     * Creates the toast element with icon and message.
     * @returns {HTMLButtonElement}
     */
    #createToastElement() {
        const toast = document.createElement("button");
        toast.className = `toast toast-${this.level}`;
        toast.setAttribute("role", "alert");
        toast.setAttribute("aria-label", "Fechar notificação");

        // Add icon
        const iconHtml = TOAST_ICONS[this.level] || TOAST_ICONS.info;
        toast.innerHTML = iconHtml;

        // Add message
        const messageSpan = document.createElement("span");
        messageSpan.className = "toast-content";
        messageSpan.textContent = this.message;
        toast.appendChild(messageSpan);

        // Add close icon
        const closeIcon = document.createElement("span");
        closeIcon.innerHTML = `<svg class="toast-close" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/>
        </svg>`;
        toast.appendChild(closeIcon);

        // Click to dismiss
        toast.addEventListener("click", () => this.dismiss());

        return toast;
    }

    /**
     * Dismisses the toast with animation.
     */
    dismiss() {
        if (this.timeoutId) {
            clearTimeout(this.timeoutId);
        }

        if (this.element) {
            this.element.classList.add("toast-hiding");
            setTimeout(() => {
                this.element.remove();
            }, 200);
        }
    }

    /**
     * Shows the toast notification.
     * @param containerSelector {string} CSS selector for the toast container.
     */
    show(containerSelector = "#toast-container") {
        const container = document.querySelector(containerSelector);
        if (!container) {
            console.warn("Toast container not found:", containerSelector);
            return;
        }

        this.element = this.#createToastElement();
        container.appendChild(this.element);

        // Auto-dismiss after duration
        this.timeoutId = setTimeout(() => this.dismiss(), TOAST_DURATION);
    }
}

/**
 * Global function to show a toast notification.
 * @param level {("info"|"success"|"warning"|"danger")}
 * @param message {string}
 */
function showToast(level, message) {
    const toast = new Toast(level, message);
    toast.show();
}

// Listen for HTMX makeToast events
document.body.addEventListener("makeToast", function(e) {
    if (e.detail && e.detail.message) {
        showToast(e.detail.level || "info", e.detail.message);
    }
});

/**
 * Reads and clears flash toast from cookie.
 */
function checkFlashToast() {
    const cookies = document.cookie.split(";");
    for (const cookie of cookies) {
        const [name, value] = cookie.trim().split("=");
        if (name === "flash_toast" && value) {
            // Clear the cookie immediately
            document.cookie = "flash_toast=; path=/; max-age=0";

            // Parse and show the toast
            const decoded = decodeURIComponent(value);
            const separatorIndex = decoded.indexOf("|");
            if (separatorIndex > 0) {
                const level = decoded.substring(0, separatorIndex);
                const message = decoded.substring(separatorIndex + 1);
                showToast(level, message);
            }
            break;
        }
    }
}

// Check for flash toast on page load
document.addEventListener("DOMContentLoaded", checkFlashToast);

// Also check after HTMX page swaps (for hx-boost navigation)
document.body.addEventListener("htmx:afterSettle", checkFlashToast);

// Export for global use
window.showToast = showToast;
