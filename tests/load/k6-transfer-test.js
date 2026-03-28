import http from 'k6/http';
import { check, sleep } from 'k6';
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
const JWT_TOKEN = __ENV.JWT_TOKEN || '';

const params = {
    headers: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${JWT_TOKEN}`
    },
};

export function setup() {
    if (!JWT_TOKEN) {
        console.error("JWT_TOKEN is required. Run with -e JWT_TOKEN=...");
    }

    // Create Sender Wallet
    let res = http.post(`${WALLET_SERVICE_URL}/api/v1/wallets`, JSON.stringify({
        user_id: uuidv4(),
        currency: 'TRY'
    }), params);

    check(res, { 'sender wallet created': (r) => r.status === 201 });
    const senderId = res.json('data.id');

    // Credit Sender Wallet with lots of money
    res = http.post(`${WALLET_SERVICE_URL}/api/v1/wallets/${senderId}/credit`, JSON.stringify({
        amount: "1000000.00",
        reference_id: uuidv4(),
        description: "Initial load test balance"
    }), params);
    check(res, { 'sender wallet credited': (r) => r.status === 200 });

    // Create Receiver Wallet
    res = http.post(`${WALLET_SERVICE_URL}/api/v1/wallets`, JSON.stringify({
        user_id: uuidv4(),
        currency: 'TRY'
    }), params);
    check(res, { 'receiver wallet created': (r) => r.status === 201 });
    const receiverId = res.json('data.id');

    return { senderId, receiverId };
}

export default function (data) {
    // Initiate Transfer
    const payload = JSON.stringify({
        sender_wallet_id: data.senderId,
        receiver_wallet_id: data.receiverId,
        amount: "1.00"
    });

    const res = http.post(`${TRANSACTION_SERVICE_URL}/api/v1/transfers`, payload, params);

    check(res, {
        'transfer accepted': (r) => r.status === 202,
        'transfer id returned': (r) => r.json('data.transfer_id') !== undefined,
    });

}
