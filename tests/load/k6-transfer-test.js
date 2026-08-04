import http from 'k6/http';
import { check } from 'k6';
import { uuidv4 } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

export const options = {
    stages: [
        { duration: '30s', target: 50 }, // Ramp up to 50 users
        { duration: '1m', target: 50 },  // Stay at 50 users
        { duration: '30s', target: 0 },  // Ramp down
    ],
    thresholds: {
        http_req_duration: ['p(99)<1500'], // 99% of requests must complete below 1.5s
        http_req_failed: ['rate<0.01'],    // Less than 1% errors
    },
};

const WALLET_SERVICE_URL = __ENV.WALLET_SERVICE_URL || 'http://localhost:8081';
const TRANSACTION_SERVICE_URL = __ENV.TRANSACTION_SERVICE_URL || 'http://localhost:8083';
const AUTH_SERVICE_URL = __ENV.AUTH_SERVICE_URL || 'http://localhost:8085';
const JWT_TOKEN = __ENV.JWT_TOKEN || '';

function params(token) {
    return {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`,
        },
    };
}

function registerFixtureUser() {
    const res = http.post(`${AUTH_SERVICE_URL}/api/v1/auth/register`, JSON.stringify({
        email: `k6-${uuidv4()}@example.test`,
        password: 'k6-integration-password',
    }), {
        headers: { 'Content-Type': 'application/json' },
    });
    check(res, { 'fixture user registered': (r) => r.status === 201 });
    return res.status === 201 ? res.json('data.access_token') : '';
}

export function setup() {
    const token = JWT_TOKEN || registerFixtureUser();
    if (!token) {
        throw new Error('unable to obtain a fixture JWT from auth-service');
    }
    const authenticatedParams = params(token);

    let res = http.post(`${WALLET_SERVICE_URL}/api/v1/wallets`, JSON.stringify({
        currency: 'TRY',
    }), authenticatedParams);

    check(res, { 'sender wallet created': (r) => r.status === 201 });
    if (res.status !== 201) {
        throw new Error(`could not create sender wallet: ${res.status}`);
    }
    const senderId = res.json('data.id');

    res = http.post(`${WALLET_SERVICE_URL}/api/v1/wallets/${senderId}/credit`, JSON.stringify({
        amount: '1000000.00',
        reference_id: uuidv4(),
        description: 'Initial load test balance',
    }), authenticatedParams);
    check(res, { 'sender wallet credited': (r) => r.status === 200 });
    if (res.status !== 200) {
        throw new Error(`could not credit sender wallet: ${res.status}`);
    }

    res = http.post(`${WALLET_SERVICE_URL}/api/v1/wallets`, JSON.stringify({
        currency: 'TRY',
    }), authenticatedParams);
    check(res, { 'receiver wallet created': (r) => r.status === 201 });
    if (res.status !== 201) {
        throw new Error(`could not create receiver wallet: ${res.status}`);
    }
    const receiverId = res.json('data.id');

    return { senderId, receiverId, token };
}

export default function (data) {
    const payload = JSON.stringify({
        sender_wallet_id: data.senderId,
        receiver_wallet_id: data.receiverId,
        amount: '1.00',
    });

    const res = http.post(`${TRANSACTION_SERVICE_URL}/api/v1/transfers`, payload, params(data.token));

    check(res, {
        'transfer accepted': (r) => r.status === 202,
        'transfer id returned': (r) => r.status === 202 && r.json('data.transfer_id') !== undefined,
    });
}
