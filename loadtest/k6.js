import http from 'k6/http';
import {check, sleep} from 'k6';

const baseURL = (__ENV.BASE_URL || 'http://127.0.0.1:8088').replace(/\/$/, '');
const accessToken = (__ENV.ACCESS_TOKEN || '').trim();
const projectCode = (__ENV.PROJECT_CODE || '').trim();
const rate = Number(__ENV.RATE || 20);
const duration = __ENV.DURATION || '30s';

export const options = {
  scenarios: {
    api_read: {
      executor: 'constant-arrival-rate',
      rate: rate,
      timeUnit: '1s',
      duration: duration,
      preAllocatedVUs: Math.max(10, Math.ceil(rate / 2)),
      maxVUs: Math.max(50, rate * 5),
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<500', 'p(99)<1000'],
    checks: ['rate>0.99'],
  },
};

function jsonOptions() {
  const headers = {'Content-Type': 'application/json'};
  if (accessToken) {
    headers.Authorization = `Bearer ${accessToken}`;
  }
  return {headers};
}

export default function () {
  const health = http.get(`${baseURL}/health`, {tags: {endpoint: 'health'}});
  check(health, {
    'health status is 200': (response) => response.status === 200,
    'health body is ok': (response) => response.json('status') === 'ok',
  });

  // Set ACCESS_TOKEN to include an authenticated read path. Do not put SMS or
  // AI calls in a load test: both are billable/external side effects.
  if (accessToken) {
    const list = http.post(
      `${baseURL}/project/project/selfList`,
      JSON.stringify({}),
      {...jsonOptions(), tags: {endpoint: 'project-self-list'}},
    );
    check(list, {
      'project list returns HTTP 200': (response) => response.status === 200,
      'project list returns a business response': (response) => {
        const body = response.json();
        return body && typeof body.code !== 'undefined';
      },
    });
  }

  if (projectCode && accessToken) {
    const detail = http.post(
      `${baseURL}/project/project/read`,
      JSON.stringify({projectCode}),
      {...jsonOptions(), tags: {endpoint: 'project-read'}},
    );
    check(detail, {
      'project detail returns HTTP 200': (response) => response.status === 200,
      'project detail returns a business response': (response) => {
        const body = response.json();
        return body && typeof body.code !== 'undefined';
      },
    });
  }

  sleep(0.1);
}
