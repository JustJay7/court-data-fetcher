// Court Data Fetcher - Frontend JavaScript

document.addEventListener('DOMContentLoaded', function() {
    // Initialize Bootstrap components
    const tooltipTriggerList = document.querySelectorAll('[data-bs-toggle="tooltip"]');
    const tooltipList = [...tooltipTriggerList].map(tooltipTriggerEl => new bootstrap.Tooltip(tooltipTriggerEl));

    // Handle search form submission
    const searchForm = document.getElementById('searchForm');
    if (searchForm) {
        searchForm.addEventListener('submit', handleSearchSubmit);
    }

    // Add input validation
    const caseNumberInput = document.getElementById('case_number');
    if (caseNumberInput) {
        caseNumberInput.addEventListener('input', function(e) {
            // Remove non-numeric characters
            e.target.value = e.target.value.replace(/[^0-9]/g, '');
        });
    }

    // Add dynamic year validation
    const filingYearSelect = document.getElementById('filing_year');
    if (filingYearSelect) {
        filingYearSelect.addEventListener('change', validateYear);
    }

    // Print functionality enhancement
    window.addEventListener('beforeprint', function() {
        document.body.classList.add('printing');
    });

    window.addEventListener('afterprint', function() {
        document.body.classList.remove('printing');
    });
});

// Handle search form submission with loading modal
function handleSearchSubmit(e) {
    const form = e.target;
    
    // Validate form
    if (!form.checkValidity()) {
        e.preventDefault();
        e.stopPropagation();
        form.classList.add('was-validated');
        return;
    }

    // Show loading modal
    const loadingModal = new bootstrap.Modal(document.getElementById('loadingModal'));
    loadingModal.show();

    // Disable submit button to prevent double submission
    const submitBtn = document.getElementById('searchBtn');
    if (submitBtn) {
        submitBtn.disabled = true;
        submitBtn.innerHTML = '<span class="spinner-border spinner-border-sm me-2"></span>Searching...';
    }
}

// Validate filing year
function validateYear(e) {
    const selectedYear = parseInt(e.target.value);
    const currentYear = new Date().getFullYear();
    
    if (selectedYear > currentYear) {
        e.target.setCustomValidity('Filing year cannot be in the future');
        showAlert('Filing year cannot be in the future', 'warning');
    } else {
        e.target.setCustomValidity('');
    }
}

// Show alert message
function showAlert(message, type = 'info') {
    const alertDiv = document.createElement('div');
    alertDiv.className = `alert alert-${type} alert-dismissible fade show`;
    alertDiv.innerHTML = `
        ${message}
        <button type="button" class="btn-close" data-bs-dismiss="alert"></button>
    `;
    
    const container = document.querySelector('main .container');
    container.insertBefore(alertDiv, container.firstChild);
    
    // Auto-dismiss after 5 seconds
    setTimeout(() => {
        alertDiv.remove();
    }, 5000);
}

// API helper functions
const API = {
    baseURL: '/api',
    
    async getCase(caseType, caseNumber, filingYear) {
        const params = new URLSearchParams({
            type: caseType,
            number: caseNumber,
            year: filingYear
        });
        
        try {
            const response = await fetch(`${this.baseURL}/case?${params}`);
            const data = await response.json();
            
            if (!response.ok) {
                throw new Error(data.error || 'Failed to fetch case data');
            }
            
            return data;
        } catch (error) {
            console.error('API Error:', error);
            throw error;
        }
    },
    
    async getCases(page = 1, limit = 10) {
        const params = new URLSearchParams({ page, limit });
        
        try {
            const response = await fetch(`${this.baseURL}/cases?${params}`);
            const data = await response.json();
            
            if (!response.ok) {
                throw new Error(data.error || 'Failed to fetch cases');
            }
            
            return data;
        } catch (error) {
            console.error('API Error:', error);
            throw error;
        }
    },
    
    async bulkSearch(queries) {
        try {
            const response = await fetch(`${this.baseURL}/cases/bulk`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify({ queries })
            });
            
            const data = await response.json();
            
            if (!response.ok) {
                throw new Error(data.error || 'Bulk search failed');
            }
            
            return data;
        } catch (error) {
            console.error('API Error:', error);
            throw error;
        }
    }
};

// Export for use in other scripts
window.CourtDataFetcher = {
    API,
    showAlert,
    validateYear
};