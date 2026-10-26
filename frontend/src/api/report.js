import $http from '@/assets/js/http'

export function weekly(data) {
    return $http.post('project/report/weekly', data);
}

export function taskSummary(data) {
    return $http.post('project/report/task-summary', data);
}
