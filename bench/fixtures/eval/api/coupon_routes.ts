import express from 'express';

const router = express.Router();

export function createCoupon(req: any, res: any) {
  res.status(201).json({ code: req.body.code });
}

router.post('/api/coupons', createCoupon);
