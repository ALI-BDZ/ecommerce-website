-- Find and delete orphaned order_items where product_name is empty/null
-- and the product no longer exists in the products table

-- Log what we're about to delete (for debugging)
DO $$
DECLARE
  r RECORD;
BEGIN
  FOR r IN
    SELECT oi.id, oi.order_id, oi.product_id, oi.product_name, oi.quantity, oi.price
    FROM order_items oi
    LEFT JOIN products p ON p.id::text = oi.product_id
    WHERE p.id IS NULL
      AND (oi.product_name IS NULL OR oi.product_name = '' OR oi.product_name = 'Unknown Product')
  LOOP
    RAISE NOTICE 'Deleting phantom order_item: id=%, order_id=%, product_id=%, name=%, qty=%, price=%',
      r.id, r.order_id, r.product_id, r.product_name, r.quantity, r.price;
  END LOOP;
END $$;

-- Delete order_items where product doesn't exist AND product_name is empty/garbage
DELETE FROM order_items oi
USING products p
WHERE p.id::text = oi.product_id
  AND (oi.product_name IS NULL OR oi.product_name = '');

-- Delete order_items where product was fully deleted and name is empty
DELETE FROM order_items oi
WHERE NOT EXISTS (SELECT 1 FROM products p WHERE p.id::text = oi.product_id)
  AND (oi.product_name IS NULL OR oi.product_name = '' OR oi.product_name = 'Unknown Product');

-- Also delete any orders that have zero items after cleanup (orphans)
DELETE FROM orders o
WHERE NOT EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id)
  AND o.status = 'pending';
