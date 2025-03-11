let searchInput;
let searchButton;
let spinner;
let resultsContainer;

addEventListener("load", (event) => {
    const buttons = document.querySelectorAll("div.w-full.relative button")
    buttons.forEach((button) => {
        button.addEventListener("click", handleSearch);
    });

    searchInput = document.querySelector("#searchInput");
    searchInput.addEventListener("keydown", function(ev) {
        if (ev.key === "Enter") {
            if (ev.target.value.length >= 1) {
                handleSearch(ev);
            }
        }
    });
    searchInput.addEventListener("input", function(ev) {
        if (ev.target.value.length >= 1) {
            enableSearchButton();
        } else {
            disableSearchButton();
        }
    });

    searchButton = document.querySelector("#searchbutton");
    spinner = document.querySelector("#spinner");
    resultsContainer = document.querySelector("#resultsContainer");
});

function handleSearch(event) {
    disableSearchButton();
    showSpinner();

    const query = searchInput.value.trim();
    if (query) {
        fetch(`/search?q=${encodeURIComponent(query)}`)
        .then((response) => {
            if (!response.ok) {
                throw new Error(`HTTP error, status ${response.status}`);
            }
            return response.text();
        })
        .then((html) => {
            // Display the results
            resultsContainer.innerHTML = html;
        })
        .catch((err) => {
            console.error('Error fetching search results: ', err);
        })
        .finally(() => {
            enableSearchButton();
            hideSpinner();
        })
    }
}

function disableSearchButton() {
    searchButton.setAttribute("disabled", true);
}

function enableSearchButton() {
    searchButton.removeAttribute("disabled");
}

function showSpinner() {
    spinner.classList.remove("hidden");
}

function hideSpinner() {
    spinner.classList.add("hidden");
}